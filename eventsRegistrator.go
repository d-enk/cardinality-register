package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"github.com/axiomhq/hyperloglog"
	"github.com/coreos/etcd/clientv3"
	"log"
	"math/rand"
	"sync"
	"time"
)

type IEventRegistrator interface {
	AddEvent(event string) error
	GetCardinality(from uint32, to uint32) (uint64, error)
}

type ApproximateEventRegistrator struct {
	sync.Mutex
	etcdClient  *clientv3.Client
	hll         *hyperloglog.Sketch
	waiters     []chan bool
	stopchan    chan struct{}
	quantumSize uint32
}

func NewApproximateRegistrator(etcdClient *clientv3.Client) *ApproximateEventRegistrator {

	s := &ApproximateEventRegistrator{}
	s.etcdClient = etcdClient

	s.hll = hyperloglog.New14()
	s.waiters = make([]chan bool, 0)

	s.stopchan = make(chan struct{})

	s.quantumSize = 10
	go s.worker()

	return s

}

func notyfyWaiters(waiters []chan bool, res bool) {
	for _, ch := range waiters {
		ch <- res
	}

}

func quantumToKey(num uint32) string {
	buf := new(bytes.Buffer)

	err := binary.Write(buf, binary.BigEndian, num)
	if err != nil {

		log.Fatal(err)
		return ""
	}
	return buf.String()
}

func (s *ApproximateEventRegistrator) saveEvents() {
	var currentHll *hyperloglog.Sketch

	s.Lock()

	if len(s.waiters) == 0 {
		s.Unlock()
		return
	}

	currentHll = s.hll
	s.hll = hyperloglog.New14()
	waiters := s.waiters
	s.waiters = make([]chan bool, 0)
	s.Unlock()

	targetHll := hyperloglog.New14()

	quant := s.getCurrentTimeQuantum()

	quantumKey := quantumToKey(quant)

	clientCtx, cancel := context.WithTimeout(context.Background(), 1000*time.Millisecond)
	resp, err := s.etcdClient.Get(clientCtx, quantumKey)
	cancel()
	if err != nil {
		notyfyWaiters(waiters, false)
		log.Println(err)
		return
	}

	var revision int64

	for {
		revision = 0

		if resp.Count == 0 {

		} else {
			binary := resp.Kvs[0].Value
			revision = resp.Kvs[0].CreateRevision
			err = targetHll.UnmarshalBinary(binary)
			if err != nil {

			}
		}

		err = targetHll.Merge(currentHll)
		if err != nil {
			targetHll = currentHll
		}

		binResult, err := targetHll.MarshalBinary()
		if err != nil {
			notyfyWaiters(waiters, false)
			log.Println(err)
			return
		}

		clientCtx, cancel := context.WithTimeout(context.Background(), 1000*time.Millisecond)

		trxResponce, err := s.etcdClient.Txn(clientCtx).
			If(clientv3.Compare(clientv3.CreateRevision(quantumKey), "=", revision)).
			Then(clientv3.OpPut(quantumKey, string(binResult))).Commit()
		cancel()

		if err != nil {
			notyfyWaiters(waiters, false)
			log.Println(err)
			return
		}

		if trxResponce.Succeeded {
			break
		} else {
			rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

			ms := int64(rnd.Uint32() % 20)

			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
	}

	res := true
	notyfyWaiters(waiters, res)

}

func (s *ApproximateEventRegistrator) worker() {

	for {
		select {
		case <-s.stopchan:
			return
		case <-time.After(time.Millisecond * 300):
			s.saveEvents()
		}
	}

}

func (s *ApproximateEventRegistrator) timestampToQuantum(timestamp uint32) uint32 {
	return uint32(timestamp / s.quantumSize)
}

func (s *ApproximateEventRegistrator) getCurrentTimeQuantum() uint32 {

	return s.timestampToQuantum(uint32(time.Now().Unix()))
}

func (s *ApproximateEventRegistrator) Shutdown() error {

	close(s.stopchan)

	return nil
}

func (s *ApproximateEventRegistrator) AddEvent(event string) error {

	ch := make(chan bool, 1)

	s.Lock()
	s.hll.Insert([]byte(event))
	s.waiters = append(s.waiters, ch)
	s.Unlock()

	res := <-ch
	close(ch)
	if !res {
		return errors.New("Event not saved")
	}

	return nil
}

func (s *ApproximateEventRegistrator) GetCardinality(from uint32, to uint32) (uint64, error) {

	fromQuantum := s.timestampToQuantum(from)
	toQuantum := s.timestampToQuantum(to)

	clientCtx, cancel := context.WithTimeout(context.Background(), 3000*time.Millisecond)

	fromKey := quantumToKey(fromQuantum)
	toKey := quantumToKey(toQuantum)
	resp, err := s.etcdClient.Get(clientCtx, fromKey, clientv3.WithRange(toKey), clientv3.WithSerializable())
	cancel()

	if err != nil {
		log.Println(err)
		return 0, err
	}

	if resp.Count == 0 {
		return 0, nil

	} else {

		totalHll := hyperloglog.New14()
		
		for _, values := range resp.Kvs {

		
			hll := hyperloglog.New14()
			err = hll.UnmarshalBinary(values.Value)
			if err != nil {
				return 0, err
			}
			err = totalHll.Merge(hll)
			if err != nil {
				return 0, err
			}

		}

		return totalHll.Estimate(), nil
	}

}
