package visualization

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ClientServerDiscovery 将服务发现返回的业务进程可视化 Server 展开为其中注册的多个 FSM。
// Discovery 中每个 endpoint 都应指向一个由 Server 启动的可视化服务，而不是单个 FSM。
type ClientServerDiscovery struct {
	Discovery ServerDiscoveryI
	Client    *http.Client
}

type clientServerTargetsResponse struct {
	Targets []clientServerTarget `json:"targets"`
}

type clientServerTarget struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
	Path        string `json:"path,omitempty"`
}

func (discovery *ClientServerDiscovery) Discover(ctx context.Context) ([]DiscoveredService, error) {
	if discovery == nil || isNilInterface(discovery.Discovery) {
		return nil, ErrInvalidDiscovery
	}
	servers, err := safeDiscover(discovery.Discovery, ctx)
	if err != nil {
		return nil, err
	}
	client := discovery.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	services := make([]DiscoveredService, 0)
	keys := make(map[string]struct{})
	var discoverErrors []error
	for _, item := range servers {
		server, normalizeErr := normalizeDiscoveredService(item)
		if normalizeErr != nil {
			discoverErrors = append(discoverErrors, normalizeErr)

			continue
		}
		var response clientServerTargetsResponse
		endpoint := remoteResourceEndpoint(server.Endpoint, "/api/targets")
		if requestErr := getRemoteJSON(ctx, client, endpoint, &response); requestErr != nil {
			discoverErrors = append(discoverErrors, fmt.Errorf("discover visualization server %q: %w", server.Key, requestErr))

			continue
		}
		for _, target := range response.Targets {
			target.ID = strings.TrimSpace(target.ID)
			if target.ID == "" || strings.ContainsAny(target.ID, "/\\") {
				discoverErrors = append(discoverErrors, fmt.Errorf("%w: invalid target id %q", ErrInvalidDiscovery, target.ID))

				continue
			}
			key := server.Key + ":" + target.ID
			if _, repeated := keys[key]; repeated {
				discoverErrors = append(discoverErrors, fmt.Errorf("%w: repeated target key %q", ErrInvalidDiscovery, key))

				continue
			}
			keys[key] = struct{}{}
			services = append(services, DiscoveredService{
				Key:            key,
				RegistrationID: remoteRegistrationID(server.Key, target.ID),
				Endpoint:       remoteResourceEndpoint(server.Endpoint, "/api/clients/"+url.PathEscape(target.ID)),
				DisplayName:    strings.TrimSpace(target.DisplayName),
				Path:           joinDiscoveryPath(server.Path, target.Path),
				PullInterval:   server.PullInterval,
			})
		}
	}
	if err := errors.Join(discoverErrors...); err != nil {
		return nil, err
	}

	return services, nil
}

func remoteRegistrationID(serverKey, targetID string) string {
	return "remote:" + url.PathEscape(serverKey) + ":" + url.PathEscape(targetID)
}

// RegisterClientServerDiscovery 注册业务进程可视化 Server 的发现源。
// 发现到 Server 后会读取其 /api/targets，并将其中的 FSM 分别注册为远程 pull ClientI。
func (server *Server) RegisterClientServerDiscovery(
	ctx context.Context,
	discovery ServerDiscoveryI,
	config DiscoveryConfig,
) (*DiscoveryRegistration, error) {
	return server.RegisterDiscovery(ctx, &ClientServerDiscovery{Discovery: discovery}, config)
}

func (server *Server) handleClientServerTargets(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)

		return
	}
	snapshots := server.Snapshots()
	targets := make([]clientServerTarget, 0, len(snapshots))
	for _, snapshot := range snapshots {
		targets = append(targets, clientServerTarget{
			ID: snapshot.ID, DisplayName: snapshot.DisplayName, Path: snapshot.Path,
		})
	}
	writeJSON(writer, http.StatusOK, clientServerTargetsResponse{Targets: targets})
}

func (server *Server) handleExportedClient(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)

		return
	}
	resource := strings.TrimPrefix(request.URL.Path, "/api/clients/")
	separator := strings.LastIndexByte(resource, '/')
	if separator <= 0 {
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": "visualization client resource not found"})

		return
	}
	id, kind := resource[:separator], resource[separator+1:]
	server.mu.RLock()
	registered, exists := server.sources[id]
	if !exists {
		server.mu.RUnlock()
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": "fsm not found"})

		return
	}
	definition := cloneDefinition(registered.definition)
	snapshot := cloneSnapshot(registered.snapshot)
	server.mu.RUnlock()

	switch kind {
	case "definition":
		writeJSON(writer, http.StatusOK, definition)
	case "realtime":
		writeJSON(writer, http.StatusOK, newRealtimeDataWire(RealtimeData{
			TransitionID: snapshot.TransitionID, CurrentState: snapshot.CurrentState,
			PreviousState: snapshot.PreviousState, PreviousEvent: snapshot.PreviousEvent,
			ProcessingEvent: snapshot.ProcessingEvent, Payload: snapshot.Payload,
			PayloadError: snapshot.PayloadError, Traces: snapshot.Traces, UpdatedAt: snapshot.UpdatedAt,
		}))
	default:
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": "visualization client resource not found"})
	}
}
