package main

import (
	"encoding/json"
	"github.com/valyala/fasthttp"
	"log"
)

type RegisterEventRequest struct {
	Event string
}

type StatusResponce struct {
	Status string
}

type CardinalityRequest struct {
	From uint32
	To   uint32
}

type CardinalityResponce struct {
	Value uint64
}


func MakeHttpServer(addr string, eventRegistrator IEventRegistrator) *fasthttp.Server {

	handler := func(ctx *fasthttp.RequestCtx) {

		switch string(ctx.Path()) {
		case "/event":
			var event RegisterEventRequest

			status := "fail"
			code := fasthttp.StatusInternalServerError

			var err error
			err = json.Unmarshal(ctx.PostBody(), &event)
			if err != nil {
				log.Println(err)
				code = fasthttp.StatusBadRequest
			} else {
				err = eventRegistrator.AddEvent(event.Event)

				if err != nil {
					log.Println(err)
				} else {
					status = "ok"
					code = fasthttp.StatusOK
				}
			}

			writeJson(ctx, code, StatusResponce{Status: status})

		case "/info":

			var infoRequest CardinalityRequest
			var info CardinalityResponce

			err := json.Unmarshal(ctx.PostBody(), &infoRequest)

			if err != nil {
				log.Println(err)
				writeJson(ctx, fasthttp.StatusBadRequest, StatusResponce{Status: "fail"})
				return
			}

			log.Println("infoRequest", infoRequest)

			info.Value = 0

			count, err := eventRegistrator.GetCardinality(infoRequest.From, infoRequest.To)
			if err != nil {
				log.Println(err)
				writeJson(ctx, fasthttp.StatusInternalServerError, StatusResponce{Status: "fail"})
				return
			}

			writeJson(ctx, fasthttp.StatusInternalServerError, CardinalityResponce{Value: count})

		default:
			ctx.Error("Unsupported path", fasthttp.StatusNotFound)
		}

	}

	s := &fasthttp.Server{
		Handler: handler,
	}

	go func() {
		err := s.ListenAndServe(addr)

		if err != nil {
			log.Println(err)
			return
		}
	}()

	return s
}

var (
	strContentType     = []byte("Content-Type")
	strApplicationJSON = []byte("application/json")
)

func writeJson(ctx *fasthttp.RequestCtx, code int, obj interface{}) {
	ctx.Response.Header.SetCanonical(strContentType, strApplicationJSON)
	ctx.Response.SetStatusCode(code)

	if err := json.NewEncoder(ctx).Encode(obj); err != nil {
		ctx.Error(err.Error(), fasthttp.StatusInternalServerError)
	}
}
