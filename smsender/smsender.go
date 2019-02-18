package smsender

import (
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/minchao/smsender/smsender/model"
	"github.com/minchao/smsender/smsender/plugin"
	"github.com/minchao/smsender/smsender/providers/notfound"
	"github.com/minchao/smsender/smsender/router"
	"github.com/minchao/smsender/smsender/store"
	"github.com/minchao/smsender/smsender/utils"
	config "github.com/spf13/viper"
	"github.com/urfave/negroni"
	"go.opencensus.io/exporter/jaeger"
	"go.opencensus.io/exporter/prometheus"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
)

type Sender struct {
	store      store.Store
	messagesCh chan *model.MessageJob
	receiptsCh chan model.MessageReceipt
	workerNum  int

	Router *router.Router
	// HTTP server router
	HTTPRouter *mux.Router
	siteURL    *url.URL

	shutdown   bool
	shutdownCh chan struct{}
	mutex      sync.RWMutex
	wg         sync.WaitGroup
}

// NewSender creates Sender.
func NewSender() *Sender {
	siteURL, err := url.Parse(config.GetString("http.siteURL"))
	if err != nil {
		log.Fatalln("config siteURL error:", err)
	}

	storeName := config.GetString("store.name")
	fn, ok := plugin.StoreFactories[storeName]
	if !ok {
		log.Fatalf("store factory '%s' not found", storeName)
	}
	s, err := fn(config.Sub(fmt.Sprintf("store.%s", storeName)))
	if err != nil {
		log.Fatalf("store init failure: %v", err)
	}
	log.Debugf("Store: %s", storeName)

	sender := &Sender{
		store:      s,
		messagesCh: make(chan *model.MessageJob, 1000),
		receiptsCh: make(chan model.MessageReceipt, 1000),
		workerNum:  config.GetInt("worker.num"),
		Router:     router.New(config.GetViper(), s, notfound.New(model.NotFoundProvider)),
		HTTPRouter: mux.NewRouter().StrictSlash(true),
		siteURL:    siteURL,
		shutdownCh: make(chan struct{}, 1),
	}

	err = sender.Router.Init()
	if err != nil {
		log.Fatalln("router init failure:", err)
	}

	return sender
}

func (s *Sender) SearchMessages(params map[string]interface{}) ([]*model.Message, error) {
	result := <-s.store.Message().Search(params)
	if result.Err != nil {
		return nil, result.Err
	}
	return result.Data.([]*model.Message), nil
}

func (s *Sender) GetMessagesByIds(ids []string) ([]*model.Message, error) {
	result := <-s.store.Message().GetByIds(ids)
	if result.Err != nil {
		return nil, result.Err
	}
	return result.Data.([]*model.Message), nil
}

func (s *Sender) GetMessagesChannel() chan *model.MessageJob {
	return s.messagesCh
}

func (s *Sender) GetSiteURL() *url.URL {
	return s.siteURL
}

// Run performs all startup actions.
func (s *Sender) Run() {
	s.InitWebhooks()
	s.InitWorkers()
	s.InitTracer()
	go s.RunHTTPServer()

	select {}
}

// Shutdown sets shutdown flag and stops all workers.
func (s *Sender) Shutdown() {
	s.mutex.Lock()
	if s.shutdown {
		s.mutex.Unlock()
		return
	}
	s.shutdown = true
	s.mutex.Unlock()

	s.wg.Add(s.workerNum)
	close(s.shutdownCh)
	s.wg.Wait()
}

// IsShutdown returns true if the server is currently shutting down.
func (s *Sender) IsShutdown() bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.shutdown
}

// InitWebhooks initializes the webhooks.
func (s *Sender) InitWebhooks() {
	for _, provider := range s.Router.GetProviders() {
		provider.Callback(
			func(webhook *model.Webhook) {
				s.HTTPRouter.HandleFunc(webhook.Path, webhook.Func).Methods(webhook.Method)
			},
			s.receiptsCh)
	}
}

// InitWorkers initializes the message workers.
func (s *Sender) InitWorkers() {
	for i := 0; i < s.workerNum; i++ {
		w := worker{i, s}
		go func(w worker) {
			for {
				select {
				case message := <-s.messagesCh:
					w.process(message)
				case receipt := <-s.receiptsCh:
					w.receipt(receipt)
				case <-s.shutdownCh:
					s.wg.Done()
					return
				}
			}
		}(w)
	}
}

// RunHTTPServer starts the HTTP server.
func (s *Sender) RunHTTPServer() {
	if !config.GetBool("http.enable") {
		return
	}
	//closer, _ := trace.Initialization("smsender", config.GetString("trace_agent"))
	//defer closer.Close()
	n := negroni.New()
	n.UseFunc(utils.Logger)
	n.UseFunc(censusMiddleware)
	//n.UseFunc(trace.Tracer)
	n.UseHandler(s.HTTPRouter)

	addr := config.GetString("http.addr")
	if config.GetBool("http.tls") {
		log.Infof("Listening for HTTPS on %s", addr)
		log.Fatal(http.ListenAndServeTLS(addr,
			config.GetString("http.tlsCertFile"),
			config.GetString("http.tlsKeyFile"),
			n))
	} else {
		log.Infof("Listening for HTTP on %s", addr)
		log.Fatal(http.ListenAndServe(addr, n))
	}
}

//InitTracer tracer
func (s *Sender) InitTracer() {
	// Register stats and trace exporters to export the collected data.
	config.SetDefault("trace_agent", "localhost:5775")
	config.BindEnv("trace_agent")
	je, err := jaeger.NewExporter(jaeger.Options{
		AgentEndpoint: config.GetString("trace_agent"),
		ServiceName:   "smsender",
	})
	if err != nil {
		log.Fatalf("Failed to create the Jaeger exporter: %v", err)
	}

	// And now finally register it as a Trace Exporter
	trace.RegisterExporter(je)

	pe, err := prometheus.NewExporter(prometheus.Options{
		Namespace: "smsender",
	})

	if err != nil {
		log.Fatalf("Failed to create Prometheus exporter: %v", err)
	}
	view.RegisterExporter(pe)

	// Always trace for this demo. In a production application, you should
	// configure this to a trace.ProbabilitySampler set at the desired
	// probability.
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})

	// Report stats at every second.
	view.SetReportingPeriod(1 * time.Second)
}

func censusMiddleware(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	h := &ochttp.Handler{Handler: next}
	h.ServeHTTP(w, r)
}
