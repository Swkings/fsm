package visualization

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

type discoveryRequest struct {
	ID                     string  `json:"id"`
	Name                   string  `json:"name,omitempty"`
	Endpoint               string  `json:"endpoint"`
	PathPrefix             string  `json:"pathPrefix,omitempty"`
	RefreshIntervalSeconds float64 `json:"refreshIntervalSeconds,omitempty"`
	PullIntervalSeconds    float64 `json:"pullIntervalSeconds,omitempty"`
}

type discoveryView struct {
	ID                     string    `json:"id"`
	Name                   string    `json:"name"`
	Endpoint               string    `json:"endpoint,omitempty"`
	PathPrefix             string    `json:"pathPrefix,omitempty"`
	RefreshIntervalSeconds float64   `json:"refreshIntervalSeconds"`
	PullIntervalSeconds    float64   `json:"pullIntervalSeconds"`
	Discovered             int       `json:"discovered"`
	Registered             int       `json:"registered"`
	LastError              string    `json:"lastError,omitempty"`
	LastRefresh            time.Time `json:"lastRefresh,omitempty"`
}

func newDiscoveryView(snapshot DiscoverySnapshot) discoveryView {
	return discoveryView{
		ID: snapshot.ID, Name: snapshot.Name, Endpoint: snapshot.Endpoint,
		PathPrefix:             snapshot.PathPrefix,
		RefreshIntervalSeconds: snapshot.RefreshInterval.Seconds(),
		PullIntervalSeconds:    snapshot.PullInterval.Seconds(),
		Discovered:             snapshot.Discovered, Registered: snapshot.Registered,
		LastError: snapshot.LastError, LastRefresh: snapshot.LastRefresh,
	}
}

func (server *Server) handleDiscoveries(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		snapshots := server.DiscoverySnapshots()
		views := make([]discoveryView, 0, len(snapshots))
		for _, snapshot := range snapshots {
			views = append(views, newDiscoveryView(snapshot))
		}
		writeJSON(writer, http.StatusOK, views)
	case http.MethodPost:
		var input discoveryRequest
		if err := decodeJSONBody(writer, request, &input); err != nil {
			writeJSON(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if input.RefreshIntervalSeconds < 0 || input.PullIntervalSeconds < 0 {
			writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "intervals must not be negative"})
			return
		}
		discovery := &HTTPServiceDiscovery{Endpoint: strings.TrimSpace(input.Endpoint)}
		registration, err := server.RegisterDiscovery(context.Background(), discovery, DiscoveryConfig{
			ID: input.ID, Name: input.Name, PathPrefix: input.PathPrefix,
			RefreshInterval: durationFromSeconds(input.RefreshIntervalSeconds),
			PullInterval:    durationFromSeconds(input.PullIntervalSeconds),
		})
		if err != nil {
			writeJSON(writer, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		views := server.DiscoverySnapshots()
		for _, snapshot := range views {
			if snapshot.ID == registration.ID() {
				writeJSON(writer, http.StatusCreated, newDiscoveryView(snapshot))
				return
			}
		}
		writeJSON(writer, http.StatusCreated, map[string]string{"id": registration.ID()})
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (server *Server) handleDiscoveryDetail(writer http.ResponseWriter, request *http.Request) {
	path := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/discoveries/"), "/")
	if path == "" {
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": ErrDiscoveryNotFound.Error()})
		return
	}
	parts := strings.Split(path, "/")
	id := parts[0]
	if len(parts) == 2 && parts[1] == "refresh" {
		if request.Method != http.MethodPost {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := server.RefreshDiscovery(request.Context(), id); err != nil {
			status := http.StatusBadGateway
			if errors.Is(err, ErrDiscoveryNotFound) {
				status = http.StatusNotFound
			}
			writeJSON(writer, status, map[string]string{"error": err.Error()})
			return
		}
		for _, snapshot := range server.DiscoverySnapshots() {
			if snapshot.ID == id {
				writeJSON(writer, http.StatusOK, newDiscoveryView(snapshot))
				return
			}
		}
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": ErrDiscoveryNotFound.Error()})
		return
	}
	if len(parts) != 1 {
		http.NotFound(writer, request)
		return
	}
	if request.Method != http.MethodDelete {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !server.UnregisterDiscovery(id) {
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": ErrDiscoveryNotFound.Error()})
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func decodeJSONBody(writer http.ResponseWriter, request *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 4<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}
