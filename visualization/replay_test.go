package visualization

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swkings/fsm/config"
	"github.com/swkings/fsm/store"
)

func TestReplayDirectoryBrowserListsDirectories(t *testing.T) {
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "archive"), 0o755); err != nil {
		t.Fatalf("create directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "ignored.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("create file: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/replay/directories?path="+url.QueryEscape(directory), nil)
	new(Server).handleReplayDirectories(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("browse directory: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var view replayDirectoryView
	if err := json.NewDecoder(recorder.Body).Decode(&view); err != nil {
		t.Fatalf("decode directory view: %v", err)
	}
	if view.Path != directory || len(view.Directories) != 1 || view.Directories[0] != "archive" {
		t.Fatalf("unexpected directory view: %+v", view)
	}
}

func TestReplaySSHDirectoryBrowserValidatesConnection(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/replay/ssh/directories", bytes.NewBufferString(`{"directory":".","ssh":{}}`))
	new(Server).handleReplaySSHDirectories(recorder, request)
	if recorder.Code != http.StatusBadGateway || !strings.Contains(recorder.Body.String(), "address is required") {
		t.Fatalf("validate SSH browser connection: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestReplayReadsFSMStoreWithoutOwningPersistence(t *testing.T) {
	backend, err := store.New(config.Store{JSONL: config.JSONL{Directory: t.TempDir(), Filename: "fsm.jsonl"}})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer backend.Close()
	record := store.Record{
		ID: "record-1", Key: "player-1", DisplayName: "Player One", Category: "examples/player",
		RecordedAt: time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC), Success: true,
		Metadata:    store.Metadata{InstanceID: "instance-1", TransitionID: "transition-1", Name: "player", PreviousState: "IDLE", PreviousEvent: "RUN", CurrentState: "RUN", Payload: []byte(`{"health":100}`)},
		Transitions: []store.Transition{{FromState: "IDLE", Event: "RUN", ToState: "RUN"}},
		Trace:       store.Trace{TransitionID: "transition-1", FromState: "IDLE", Event: "RUN", ToState: "RUN", FinalState: "RUN", Success: true},
	}
	if err = backend.Save(context.Background(), record); err != nil {
		t.Fatalf("save record: %v", err)
	}

	hub := newReplayHub()
	defer hub.close()
	if err = hub.register("primary", "FSM Store", backend, true, false); err != nil {
		t.Fatalf("register replay store: %v", err)
	}
	records, err := hub.list(context.Background(), "primary", "player-1", 10)
	if err != nil || len(records) != 1 {
		t.Fatalf("list replay records: records=%+v err=%v", records, err)
	}
	if records[0].Trace.Event != "RUN" || records[0].Trace.FromState != "IDLE" || records[0].Trace.FinalState != "RUN" {
		t.Fatalf("list replay trace: %+v", records[0].Trace)
	}
	replay, err := hub.get(context.Background(), "primary", record.ID)
	if err != nil {
		t.Fatalf("get replay record: %v", err)
	}
	if replay.Snapshot.CurrentState != "RUN" || replay.Snapshot.PreviousEvent != "RUN" || len(replay.Snapshot.Transitions) != 1 {
		t.Fatalf("unexpected replay snapshot: %+v", replay.Snapshot)
	}
}
