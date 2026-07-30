package visualization

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/swkings/fsm/config"
	"github.com/swkings/fsm/store"
)

var (
	ErrReplaySourceNotFound = errors.New("replay data source not found")
	ErrReplayRecordNotFound = store.ErrNotFound
)

type ReplaySourceConfig = config.StoreSource

type ReplaySourceView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Driver    string `json:"driver"`
	Location  string `json:"location"`
	Primary   bool   `json:"primary"`
	LastError string `json:"lastError,omitempty"`
}

type ReplayRecord struct {
	ID          string    `json:"id"`
	SourceID    string    `json:"sourceId,omitempty"`
	FSMID       string    `json:"fsmId"`
	DisplayName string    `json:"displayName"`
	Path        string    `json:"path,omitempty"`
	RecordedAt  time.Time `json:"recordedAt"`
	Snapshot    Snapshot  `json:"snapshot"`
}

type ReplayRecordSummary struct {
	ID           string        `json:"id"`
	SourceID     string        `json:"sourceId"`
	FSMID        string        `json:"fsmId"`
	DisplayName  string        `json:"displayName"`
	Path         string        `json:"path,omitempty"`
	TransitionID string        `json:"transitionId,omitempty"`
	State        string        `json:"state"`
	Event        string        `json:"event,omitempty"`
	Success      bool          `json:"success"`
	RecordedAt   time.Time     `json:"recordedAt"`
	Trace        TraceSnapshot `json:"trace"`
}

type replaySource struct {
	id, name string
	backend  store.StoreI
	primary  bool
	owned    bool
	lastErr  string
	mu       sync.RWMutex
}

func (source *replaySource) fail(err error) {
	source.mu.Lock()
	if err == nil {
		source.lastErr = ""
	} else {
		source.lastErr = err.Error()
	}
	source.mu.Unlock()
}

func (source *replaySource) view() ReplaySourceView {
	source.mu.RLock()
	defer source.mu.RUnlock()
	descriptor := source.backend.Descriptor()
	return ReplaySourceView{ID: source.id, Name: source.name, Driver: descriptor.Driver, Location: descriptor.Location, Primary: source.primary, LastError: source.lastErr}
}

type replayHub struct {
	mu      sync.RWMutex
	sources map[string]*replaySource
}

func newReplayHub() *replayHub { return &replayHub{sources: map[string]*replaySource{}} }

func validReplaySourceID(id string) bool {
	return id != "" && !strings.ContainsAny(id, `/\\`)
}

func (hub *replayHub) register(id, name string, backend store.StoreI, primary, owned bool) error {
	id = strings.TrimSpace(id)
	if !validReplaySourceID(id) {
		return fmt.Errorf("invalid replay source id %q", id)
	}
	if backend == nil {
		return errors.New("replay store is nil")
	}
	if strings.TrimSpace(name) == "" {
		name = id
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if _, exists := hub.sources[id]; exists {
		return errors.New("replay source already exists")
	}
	hub.sources[id] = &replaySource{id: id, name: name, backend: backend, primary: primary, owned: owned}
	return nil
}

func (hub *replayHub) add(_ context.Context, cfg ReplaySourceConfig) (ReplaySourceView, error) {
	backend, err := store.New(cfg.Store)
	if err != nil {
		return ReplaySourceView{}, err
	}
	if err = hub.register(cfg.ID, cfg.Name, backend, cfg.ID == "primary", true); err != nil {
		_ = backend.Close()
		return ReplaySourceView{}, err
	}
	source, _ := hub.source(cfg.ID)
	return source.view(), nil
}

func (hub *replayHub) source(id string) (*replaySource, error) {
	if id == "" {
		id = "primary"
	}
	hub.mu.RLock()
	source, exists := hub.sources[id]
	hub.mu.RUnlock()
	if !exists {
		return nil, ErrReplaySourceNotFound
	}
	return source, nil
}

func (hub *replayHub) drop(id string) bool {
	hub.mu.Lock()
	source, exists := hub.sources[id]
	if !exists || source.primary {
		hub.mu.Unlock()
		return false
	}
	delete(hub.sources, id)
	hub.mu.Unlock()
	if source.owned {
		_ = source.backend.Close()
	}
	return true
}

func (hub *replayHub) views() []ReplaySourceView {
	hub.mu.RLock()
	views := make([]ReplaySourceView, 0, len(hub.sources))
	for _, source := range hub.sources {
		views = append(views, source.view())
	}
	hub.mu.RUnlock()
	sort.Slice(views, func(i, j int) bool {
		if views[i].Primary != views[j].Primary {
			return views[i].Primary
		}
		return views[i].Name < views[j].Name
	})
	return views
}

func (hub *replayHub) list(ctx context.Context, sourceID, key string, limit int) ([]ReplayRecordSummary, error) {
	source, err := hub.source(sourceID)
	if err != nil {
		return nil, err
	}
	records, err := source.backend.List(ctx, store.Query{Key: key, Limit: limit})
	source.fail(err)
	if err != nil {
		return nil, err
	}
	summaries := make([]ReplayRecordSummary, 0, len(records))
	for _, record := range records {
		summaries = append(summaries, ReplayRecordSummary{
			ID: record.ID, SourceID: source.id, FSMID: record.Key,
			DisplayName: record.DisplayName, Path: record.Category,
			TransitionID: record.Metadata.TransitionID, State: string(record.Metadata.CurrentState),
			Event: record.Trace.Event, Success: record.Success, RecordedAt: record.RecordedAt,
			Trace: storeTraceSnapshot(record.Trace),
		})
	}
	return summaries, nil
}

func (hub *replayHub) get(ctx context.Context, sourceID, id string) (ReplayRecord, error) {
	source, err := hub.source(sourceID)
	if err != nil {
		return ReplayRecord{}, err
	}
	record, err := source.backend.Get(ctx, id)
	source.fail(err)
	if err != nil {
		return ReplayRecord{}, err
	}
	return replayRecord(source.id, record), nil
}

func (hub *replayHub) remove(ctx context.Context, sourceID, id string) error {
	source, err := hub.source(sourceID)
	if err != nil {
		return err
	}
	err = source.backend.Delete(ctx, id)
	source.fail(err)
	return err
}

func (hub *replayHub) close() {
	hub.mu.Lock()
	sources := hub.sources
	hub.sources = map[string]*replaySource{}
	hub.mu.Unlock()
	for _, source := range sources {
		if source.owned {
			_ = source.backend.Close()
		}
	}
}

func replayRecord(sourceID string, record store.Record) ReplayRecord {
	snapshot := storeRecordSnapshot(record)
	return ReplayRecord{ID: record.ID, SourceID: sourceID, FSMID: record.Key, DisplayName: record.DisplayName, Path: record.Category, RecordedAt: record.RecordedAt, Snapshot: snapshot}
}

func storeRecordSnapshot(record store.Record) Snapshot {
	metadata := record.Metadata
	transitions := make([]TransitionSnapshot, 0, len(record.Transitions))
	statesSet, eventsSet := map[string]struct{}{}, map[string]struct{}{}
	for _, transition := range record.Transitions {
		item := TransitionSnapshot(transition)
		transitions = append(transitions, item)
		statesSet[string(item.FromState)], statesSet[string(item.ToState)], eventsSet[string(item.Event)] = struct{}{}, struct{}{}, struct{}{}
	}
	states, events := make([]string, 0, len(statesSet)), make([]string, 0, len(eventsSet))
	for state := range statesSet {
		states = append(states, state)
	}
	for event := range eventsSet {
		events = append(events, event)
	}
	sort.Strings(states)
	sort.Strings(events)
	available := make([]TransitionSnapshot, 0)
	for _, transition := range transitions {
		if transition.FromState == metadata.CurrentState {
			available = append(available, transition)
		}
	}
	var traces []TraceSnapshot
	if record.Trace.TransitionID != "" || record.Trace.Event != "" {
		traces = []TraceSnapshot{storeTraceSnapshot(record.Trace)}
	}
	displayName := record.DisplayName
	if displayName == "" {
		displayName = string(metadata.Name)
	}
	snapshot := Snapshot{
		Descriptor: Descriptor{ID: metadata.InstanceID, Name: string(metadata.Name), DisplayName: displayName, Description: metadata.Description, Path: record.Category},
		Mode:       ModePush, TransitionID: metadata.TransitionID, CurrentState: string(metadata.CurrentState),
		PreviousState: string(metadata.PreviousState), PreviousEvent: string(metadata.PreviousEvent),
		Payload: append([]byte(nil), metadata.Payload...), States: states, Events: events,
		Transitions: transitions, AvailableTransitions: available, Traces: traces, UpdatedAt: record.RecordedAt,
	}
	if metadata.PayloadError != "" {
		snapshot.PayloadError = errors.New(metadata.PayloadError)
	}
	return snapshot
}

func storeTraceSnapshot(trace store.Trace) TraceSnapshot {
	duration := time.Duration(0)
	if trace.EndTime > trace.BeginTime {
		duration = time.Duration(trace.EndTime - trace.BeginTime)
	}
	return TraceSnapshot{
		TransitionID: trace.TransitionID, PreviousID: trace.PreviousID,
		FromState: trace.FromState, Event: trace.Event, ToState: trace.ToState,
		FinalState: trace.FinalState, Success: trace.Success, Error: trace.Error,
		EntryMetadata: storeMetadataSnapshot(trace.Entry), ExitMetadata: storeMetadataSnapshot(trace.Exit),
		BeginTime: trace.BeginTime, EndTime: trace.EndTime, Duration: duration,
	}
}

func storeMetadataSnapshot(metadata store.Metadata) MetadataSnapshot {
	return metadata.Clone()
}

// RegisterReplayStore 将 FSM Store 暴露给 Replay 页面。Store 生命周期仍由调用方管理。
func (server *Server) RegisterReplayStore(id, name string, backend store.StoreI) error {
	return server.replays.register(id, name, backend, id == "primary", false)
}

// UnregisterReplayStore 从 Replay 页面移除非 primary 数据源。
func (server *Server) UnregisterReplayStore(id string) bool { return server.replays.drop(id) }
