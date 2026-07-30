package visualization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ServiceDiscoveryFunc func(context.Context) ([]DiscoveredService, error)

func (discover ServiceDiscoveryFunc) Discover(ctx context.Context) ([]DiscoveredService, error) {
	return discover(ctx)
}

// HTTPServiceDiscovery 自动识别注册中心服务列表接口或单个 ClientI Server 基础地址。
type HTTPServiceDiscovery struct {
	Endpoint  string
	DirectKey string
	Client    *http.Client
}

type httpDiscoveryResponse struct {
	Services []httpDiscoveredService `json:"services"`
}

type httpDiscoveredService struct {
	Key                 string  `json:"key,omitempty"`
	RegistrationID      string  `json:"registrationId,omitempty"`
	Endpoint            string  `json:"endpoint"`
	DisplayName         string  `json:"displayName,omitempty"`
	Path                string  `json:"path,omitempty"`
	PullIntervalSeconds float64 `json:"pullIntervalSeconds,omitempty"`
}

func (discovery *HTTPServiceDiscovery) Discover(ctx context.Context) ([]DiscoveredService, error) {
	if discovery == nil {
		return nil, ErrInvalidDiscovery
	}
	endpoint := strings.TrimRight(strings.TrimSpace(discovery.Endpoint), "/")
	if err := validateRemoteEndpoint(endpoint); err != nil {
		return nil, err
	}
	client := discovery.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	var response httpDiscoveryResponse
	registryErr := getRemoteJSON(ctx, client, endpoint, &response)
	if registryErr == nil && response.Services != nil {
		services := make([]DiscoveredService, 0, len(response.Services))
		for _, item := range response.Services {
			if item.PullIntervalSeconds < 0 {
				return nil, fmt.Errorf("%w: negative pull interval for %q", ErrInvalidDiscovery, item.Key)
			}
			services = append(services, DiscoveredService{
				Key: item.Key, RegistrationID: item.RegistrationID,
				Endpoint: item.Endpoint, DisplayName: item.DisplayName, Path: item.Path,
				PullInterval: durationFromSeconds(item.PullIntervalSeconds),
			})
		}

		return services, nil
	}

	var targets clientServerTargetsResponse
	directEndpoint := remoteResourceEndpoint(endpoint, "/api/targets")
	if directErr := getRemoteJSON(ctx, client, directEndpoint, &targets); directErr == nil {
		key := strings.TrimSpace(discovery.DirectKey)
		if key == "" {
			key = endpoint
		}

		return []DiscoveredService{{Key: key, Endpoint: endpoint}}, nil
	} else {
		if registryErr == nil {
			registryErr = errors.New("discovery response does not contain services")
		}

		return nil, fmt.Errorf(
			"discover visualization servers from %s: %w",
			endpoint,
			errors.Join(registryErr, fmt.Errorf("probe direct client server: %w", directErr)),
		)
	}
}

type remotePullClient struct {
	ctx            context.Context
	endpoint       string
	registrationID string
	client         *http.Client
}

func (client *remotePullClient) VisualizationDefinition() Definition {
	var definition Definition
	if err := getRemoteJSON(client.ctx, client.client, remoteResourceEndpoint(client.endpoint, "/definition"), &definition); err != nil {
		panic(err)
	}
	if client.registrationID != "" {
		definition.ID = client.registrationID
	}

	return definition
}

func (client *remotePullClient) VisualizationSnapshot(ctx context.Context) (RealtimeData, error) {
	var wire realtimeDataWire
	if err := getRemoteJSON(ctx, client.client, remoteResourceEndpoint(client.endpoint, "/realtime"), &wire); err != nil {
		return RealtimeData{}, err
	}

	return wire.realtimeData(), nil
}

func getRemoteJSON(ctx context.Context, client *http.Client, endpoint string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))

		return fmt.Errorf("remote endpoint returned %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4<<20))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode remote response: %w", err)
	}

	return nil
}

type realtimeDataWire struct {
	TransitionID    string          `json:"transitionId"`
	CurrentState    string          `json:"currentState"`
	PreviousState   string          `json:"previousState"`
	PreviousEvent   string          `json:"previousEvent"`
	ProcessingEvent string          `json:"processingEvent,omitempty"`
	Payload         json.RawMessage `json:"payload"`
	PayloadError    string          `json:"payloadError,omitempty"`
	Traces          []TraceSnapshot `json:"traces,omitempty"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

func newRealtimeDataWire(data RealtimeData) realtimeDataWire {
	payloadError := ""
	if data.PayloadError != nil {
		payloadError = data.PayloadError.Error()
	}

	return realtimeDataWire{
		TransitionID: data.TransitionID, CurrentState: data.CurrentState,
		PreviousState: data.PreviousState, PreviousEvent: data.PreviousEvent,
		ProcessingEvent: data.ProcessingEvent, Payload: data.Payload,
		PayloadError: payloadError, Traces: data.Traces, UpdatedAt: data.UpdatedAt,
	}
}

func (wire realtimeDataWire) realtimeData() RealtimeData {
	var payloadError error
	if wire.PayloadError != "" {
		payloadError = errors.New(wire.PayloadError)
	}

	return RealtimeData{
		TransitionID: wire.TransitionID, CurrentState: wire.CurrentState,
		PreviousState: wire.PreviousState, PreviousEvent: wire.PreviousEvent,
		ProcessingEvent: wire.ProcessingEvent, Payload: wire.Payload,
		PayloadError: payloadError, Traces: wire.Traces, UpdatedAt: wire.UpdatedAt,
	}
}

func NewClientHandler(client ClientI) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/definition", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.WriteHeader(http.StatusMethodNotAllowed)

			return
		}
		if isNilInterface(client) {
			writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"error": ErrInvalidClient.Error()})

			return
		}
		definition, err := clientDefinition(client)
		if err != nil {
			writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": err.Error()})

			return
		}
		writeJSON(writer, http.StatusOK, definition)
	})
	mux.HandleFunc("/realtime", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.WriteHeader(http.StatusMethodNotAllowed)

			return
		}
		if isNilInterface(client) {
			writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"error": ErrInvalidClient.Error()})

			return
		}
		data, err := clientRealtimeData(client, request.Context())
		if err != nil {
			writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})

			return
		}
		writeJSON(writer, http.StatusOK, newRealtimeDataWire(data))
	})

	return mux
}

func durationFromSeconds(seconds float64) time.Duration {
	if seconds <= 0 {
		return 0
	}

	return time.Duration(seconds * float64(time.Second))
}
