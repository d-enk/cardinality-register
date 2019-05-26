package main

import (
	"flag"
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/embed"
)

var (
	httpAddr           = flag.String("httpAddr", ":8080", "TCP address to listen to")
	storeDir           = flag.String("storeDir", "/var/lib/default.etcd", "Directory for store data")
	nodeName           = flag.String("nodeName", "node1", "Node name for cluster")
	etcdInitialCluster = flag.String("etcdInitialCluster", "", "Etcd initial cluster option")
	etcdClusterState   = flag.String("etcdClusterState", "new", "Etcd cluster state")
)

func main() {
	flag.Parse()

	cfg := embed.NewConfig()
	cfg.Dir = *storeDir

	if *etcdInitialCluster != "" {

		cfg.InitialCluster = *etcdInitialCluster
		cfg.ClusterState = *etcdClusterState

		apurl, _ := url.Parse("http://" + *nodeName + ":2380")
		acurl, _ := url.Parse("http://" + *nodeName + ":2379")

		lpurl, _ := url.Parse("http://0.0.0.0:2380")
		lcurl, _ := url.Parse("http://0.0.0.0:2379")

		cfg.APUrls = []url.URL{*apurl}
		cfg.ACUrls = []url.URL{*acurl}

		cfg.LPUrls = []url.URL{*lpurl}
		cfg.LCUrls = []url.URL{*lcurl}

		cfg.Name = *nodeName
	}

	err := cfg.Validate()
	if err != nil {
		log.Fatal(err)
	}

	etcdServer, err := embed.StartEtcd(cfg)
	if err != nil {
		log.Fatal(err)
	}

	defer etcdServer.Close()
	select {
	case <-etcdServer.Server.ReadyNotify():
		log.Printf("Server is ready!")
	case <-time.After(60 * time.Second):
		etcdServer.Server.Stop() // trigger a shutdown
		log.Printf("Server took too long to start!")
	}

	etcdCli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	})

	defer etcdCli.Close()

	registrator := NewApproximateRegistrator(etcdCli)

	httpServer := MakeHttpServer(*httpAddr, registrator)

	if httpServer == nil {
		log.Fatalf("Error in ListenAndServe: %s", err)
	}

	stop := make(chan os.Signal, 1)

	signal.Notify(stop, os.Interrupt, os.Kill)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGILL, syscall.SIGINT)

	sig := <-stop

	log.Println("Start shutdown process", sig)

	err = registrator.Shutdown()
	if err != nil {
		log.Println("Error in registrator.Shutdown() ", err)
	} else {
		log.Println("Successfully registrator.Shutdown() ")
	}

	err = httpServer.Shutdown()
	if err != nil {
		log.Println("Error in httpServer.Shutdown() ", err)
	} else {
		log.Println("Successfully httpServer.Shutdown()")
	}

}
