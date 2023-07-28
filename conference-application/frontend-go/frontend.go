package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/open-feature/go-sdk/pkg/openfeature"
	"github.com/salaboy/platforms-on-k8s/conference-application/frontend-go/api"
)

var (
	Version = getEnv("VERSION", "2.0.0")

	Source                  = getEnv("SOURCE", "https://github.com/salaboy/platforms-on-k8s/tree/v2.0.0/conference-application/frontend-go")
	PodName                 = getEnv("POD_NAME", "N/A")
	PodNamespace            = getEnv("POD_NAMESPACE", "N/A")
	PodNodeName             = getEnv("POD_NODENAME", "N/A")
	PodIp                   = getEnv("POD_IP", "N/A")
	PodServiceAccount       = getEnv("POD_SERVICE_ACCOUNT", "N/A")
	AgendaServiceUrl        = getEnv("AGENDA_SERVICE_URL", "http://agenda-service.default.svc.cluster.local")
	C4pServiceUrl           = getEnv("C4P_SERVICE_URL", "http://c4p-service.default.svc.cluster.local")
	NotificationsServiceUrl = getEnv("NOTIFICATIONS_SERVICE_URL", "http://notifications-service.default.svc.cluster.local")

	AppPort   = getEnv("APP_PORT", "8080")
	FlagdHost = getEnv("FLAGD_HOST", "http://flagd.default.svc.cluster.local")
	FlagdPort = getEnv("FLAGD_PORT", "8013")
	// FeatureGenerateProposal values:
	// - PUBLIC (no filters)
	// - GENERATE (Read Only Form - Generate Proposal)
	// -GENERATE_ONLY (No Submit until Generated Proposal is created)
	FeatureGenerateProposal = getEnv("FEATURE_DEBUG_ENABLED", "GENERATE")
	FeatureDebugEnabled     = getEnv("FEATURE_DEBUG_ENABLED", "false")
	KoDataPath              = getEnv("KO_DATA_PATH", "kodata")
)

const (
	ApplicationJson = "application/json"
	ContentType     = "Content-Type"
)

type ServiceInfo struct {
	Name              string `json:"name"`
	Version           string `json:"version"`
	Source            string `json:"source"`
	PodName           string `json:"podName"`
	PodNamespace      string `json:"podNamespace"`
	PodNodeName       string `json:"podNodeName"`
	PodIp             string `json:"podIp"`
	PodServiceAccount string `json:"podServiceAccount"`
}

type Event struct {
	Id      string `json:"id"`
	Payload string `json:"payload"`
	Type    string `json:"type"`
}

type Features struct {
	DebugEnabled     string
	GenerateProposal string
}

var events = []Event{}

func main() {

	r := NewChiServer()

	openfeature.SetProvider(flagd.NewProvider(
		flagd.WithHost(FlagdHost),
		flagd.WithPort(FlagdPort),
	))

	client := openfeature.NewClient("frontend")

	value, err := client.BooleanValue(
		context.Background(), "debug_enabled", false, openfeature.EvaluationContext{},
	)

	log.Printf("Starting Frontend Go in Port: %s", AppPort)

	// Start the server; this is a blocking call
	err = http.ListenAndServe(":"+AppPort, r)
	if err != http.ErrServerClosed {
		log.Panic(err)
	}
}

func featureHandler(w http.ResponseWriter, r *http.Request) {
	var features = Features{
		DebugEnabled:     FeatureDebugEnabled,
		GenerateProposal: FeatureGenerateProposal,
	}
	respondWithJSON(w, http.StatusOK, features)
}

func eventsHandler(w http.ResponseWriter, r *http.Request) {
	respondWithJSON(w, http.StatusOK, events)
}

func agendaServiceHandler(w http.ResponseWriter, r *http.Request) {
	proxyRequest("api/agenda", AgendaServiceUrl, w, r)
}

func c4PServiceHandler(w http.ResponseWriter, r *http.Request) {
	proxyRequest("api/c4p", C4pServiceUrl, w, r)
}

func notificationServiceHandler(w http.ResponseWriter, r *http.Request) {
	proxyRequest("api/notifications", NotificationsServiceUrl, w, r)
}

func proxyRequest(serviceName string, serviceUrl string, w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	r.Body = io.NopCloser(bytes.NewReader(body))

	requestDump, err := httputil.DumpRequest(r, true)
	if err != nil {
		fmt.Println(err)
	}
	log.Println(string(requestDump))

	url := fmt.Sprintf("%s%s", serviceUrl, r.RequestURI)
	if !strings.HasPrefix(url, "http") {
		url = fmt.Sprintf("http://%s", url)
	}

	log.Printf("Proxying request before replace to %s", url)
	// remove the service path
	url = strings.Replace(url, serviceName+"/", "", -1)

	log.Printf("Proxying request to %s", url)

	proxyReq, err := http.NewRequest(r.Method, url, bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	proxyReq.Header = make(http.Header)
	for h, val := range r.Header {
		proxyReq.Header[h] = val
	}

	resp, err := http.DefaultClient.Do(proxyReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for h, val := range resp.Header {
		w.Header()[h] = val
	}

	w.WriteHeader(resp.StatusCode)

	log.Printf("Proxied request response code %s - %d", resp.Status, resp.StatusCode)

	_, err = w.Write(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

// getEnv is a helper function to get environment variable or return a default value.
func getEnv(key, fallback string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		value = fallback
	}
	return value
}

// OpenAPI OpenAPIHandler returns a handler that serves the OpenAPI documentation.
func OpenAPI(r *chi.Mux) {
	fs := http.FileServer(http.Dir(KoDataPath + "/docs/"))
	r.Handle("/openapi/*", http.StripPrefix("/openapi/", fs))
}

// server implements api.ServerInterface interface.
type server struct{}

// GetEventsWithPost gets all events from the in-memory store.
func (s *server) GetEventsWithPost(w http.ResponseWriter, r *http.Request) {
	respondWithJSON(w, http.StatusOK, events)
}

// GetEventsWithGet gets all events from the in-memory store.
func (s *server) GetEventsWithGet(w http.ResponseWriter, r *http.Request) {
	respondWithJSON(w, http.StatusOK, events)
}

// GetServiceInfo gets service information.
func (s *server) GetServiceInfo(w http.ResponseWriter, r *http.Request) {
	var info = ServiceInfo{
		Name:              "FRONTEND",
		Version:           Version,
		Source:            Source,
		PodName:           PodName,
		PodNodeName:       PodNodeName,
		PodNamespace:      PodNamespace,
		PodIp:             PodIp,
		PodServiceAccount: PodServiceAccount,
	}
	w.Header().Set(ContentType, ApplicationJson)
	json.NewEncoder(w).Encode(info)
}

// NewServer creates a new api.ServerInterface server.
func NewServer() api.ServerInterface {
	return &server{}
}

// NewChiServer creates a new chi.Mux server.
func NewChiServer() *chi.Mux {

	r := chi.NewRouter()

	r.Use(middleware.Logger)

	fs := http.FileServer(http.Dir(KoDataPath))

	server := NewServer()

	OpenAPI(r)

	r.HandleFunc("/api/agenda/*", agendaServiceHandler)
	r.HandleFunc("/api/c4p/*", c4PServiceHandler)
	r.HandleFunc("/api/notifications/*", notificationServiceHandler)
	r.HandleFunc("/api/features/*", featureHandler)

	r.Mount("/api/", api.Handler(server))
	r.Handle("/*", http.StripPrefix("/", fs))

	// Add handlers for readiness and liveness endpoints
	r.HandleFunc("/health/{endpoint:readiness|liveness}", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})

	return r
}

func (s *server) NewEvents(w http.ResponseWriter, r *http.Request) {
	cloudEvent, err := cloudevents.NewEventFromHTTPRequest(r)
	if err != nil {
		log.Printf("failed to parse CloudEvent from request: %v", err)

	}
	log.Println(cloudEvent.String())
	var event Event
	err = json.Unmarshal(cloudEvent.DataEncoded, &event)
	if err != nil {
		log.Printf("failed to parse Event Data from CloudEvent: %v", err)

	}
	fmt.Printf("Event received (type: %s): %s, Payload: %s \n", event.Type, event.Id, event.Payload)
	events = append(events, event)
	respondWithJSON(w, http.StatusOK, event)
}
