package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"k8s.io/utils/env"
)

type apiHandler struct {
	Event  string
	Schema *jsonschema.Schema
	Js     jetstream.JetStream
}

type MerchantData struct {
	Data []interface{}
}

func (a *apiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	event := MerchantData{}
	fmt.Printf("serving event %s ...\n", a.Event)
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "Request body illegal", http.StatusBadRequest)
		return
	}
	for _, e := range event.Data {
		if err := a.Schema.Validate(e); err != nil {
			fmt.Printf("%v", err)
			http.Error(w, "Request body illegal", http.StatusBadRequest)
		}
		buff := make([]byte, 0)
		s := bytes.NewBuffer(buff)
		if err := json.NewEncoder(s).Encode(&event.Data); err != nil {
			http.Error(w, "Request body illegal", http.StatusBadRequest)
			return
		}
		_, err := a.Js.Publish(context.Background(), a.Event, s.Bytes())
		if err != nil {
			fmt.Printf("%v\n", err)
			http.Error(w, "Error", http.StatusInternalServerError)
		} else {
			fmt.Println("client ok")
			fmt.Fprintf(w, "OKKK!")
		}
		fmt.Println("finished serving event ...")
	}
}

func main() {
	event := env.GetString("event", "")
	if event == "" {
		event = "def"
	}
	schema := env.GetString("schema", "")
	if schema == "" {
		panic("no schema")
	}
	fmt.Printf("started with event %s and schema %s\n", event, schema)
	// Nats
	url := env.GetString("NATS_URL", "nats://queue:4222")
	fmt.Printf("v1: Connecting Nats with %s\n", url)
	nc, err := nats.Connect(url)
	fmt.Println("connected.")
	if err != nil {
		panic(err)
	}
	defer nc.Drain()
	js, err := jetstream.New(nc)
	if err != nil {
		panic(err)
	}
	cfg := jetstream.StreamConfig{
		Name:      "EVENTS-" + event,
		Retention: jetstream.WorkQueuePolicy,
		Subjects:  []string{event},
	}

	// JetStream API uses context for timeouts and cancellation.
	fmt.Printf("about to create the stream.")
	_, err = js.CreateOrUpdateStream(context.Background(), cfg)
	if err != nil {
		panic(err)
	}
	fmt.Println("created the stream")
	//
	res := fmt.Sprintf("/listener/%s", event)
	fmt.Printf("listening on %s\n", res)
	mux := http.NewServeMux()

	comp := jsonschema.NewCompiler()
	file := "/etc/listener/" + schema
	fmt.Printf("reading file: %s\n", file)
	bs, err := os.ReadFile(file)
	if err != nil {
		panic(err)
	}
	if err := comp.AddResource("schema", bytes.NewReader(bs)); err != nil {
		panic(err)
	}
	compiledSchema, err := comp.Compile("schema")
	if err != nil {
		panic(err)
	}
	h := apiHandler{
		Schema: compiledSchema,
		Js:     js,
		Event:  event,
	}
	mux.Handle(res, &h)
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			http.NotFound(w, req)
			return
		}
		fmt.Fprintf(w, "Service for %s!", event)
	})

	var srv http.Server
	closed := make(chan struct{})
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint

		if err := srv.Shutdown(context.Background()); err != nil {
			fmt.Printf("HTTP server Shutdown: %v", err)
		}
		close(closed)
	}()

	srv = http.Server{Addr: ":8080", Handler: mux}
	fmt.Printf("listening %s on 8080\n", event)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		fmt.Printf("HTTP server ListenAndServe: %v", err)
	}

	<-closed
	println("stopped")
}
