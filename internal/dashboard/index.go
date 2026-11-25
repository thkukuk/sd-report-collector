package dashboard

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"sd-report-collector/internal/config"
)

//go:embed index_template.html
var indexTemplate []byte

// indexStateFile is where the overview index persists what it knows about
// each host between invocations (one plugin invocation only sees a single
// host's report), so it can still list hosts that have gone quiet. It's
// named so it never collides with a "<hostname>.html" dashboard.
const indexStateFile = ".index-state.json"

// indexLockFile serializes updateIndex's read-modify-write of indexStateFile
// across the separate sd-report-dashboard processes sd-report-collector
// spawns concurrently for reports arriving from different hosts at nearly
// the same time; without it, two processes racing to read, update and
// rewrite the same state file would silently drop one host's update.
const indexLockFile = ".index.lock"

// indexDataFile holds the same payload embedded in index.html, as its own
// static file. index.html's JS polls this periodically and merges it in, so
// a browser tab left open keeps learning about new reports instead of
// judging staleness forever against the data it happened to load with.
const indexDataFile = "index-data.json"

// nodeStatus is one host's last-known state, as tracked in indexStateFile
// and rendered into the overview index.
type nodeStatus struct {
	Hostname    string    `json:"hostname"`
	LastSeen    time.Time `json:"lastSeen"`
	SignStatus  string    `json:"signStatus"`
	SystemState string    `json:"systemState"`
	Metrics     int       `json:"metrics"`
}

// updateIndex records hostname's latest report in indexStateFile and
// re-renders index.html from the full set of known hosts. A failure here is
// logged by the caller but must not take down the per-host dashboard write
// that already succeeded.
func updateIndex(cfg *config.DashboardConfig, hostname, signStatus string, reportJSON []byte, reportTime time.Time) error {
	lock, err := os.OpenFile(filepath.Join(cfg.OutputDirectory, indexLockFile), os.O_CREATE|os.O_RDWR, 0o640)
	if err != nil {
		return fmt.Errorf("opening index lock: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("locking index: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	statePath := filepath.Join(cfg.OutputDirectory, indexStateFile)

	states, err := loadIndexState(statePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", statePath, err)
	}

	if reportTime.IsZero() {
		reportTime = time.Now().UTC()
	}
	states[hostname] = nodeStatus{
		Hostname:    hostname,
		LastSeen:    reportTime.UTC(),
		SignStatus:  signStatus,
		SystemState: extractSystemState(reportJSON),
		Metrics:     countMetrics(reportJSON),
	}

	if err := saveIndexState(statePath, states); err != nil {
		return fmt.Errorf("writing %s: %w", statePath, err)
	}

	dataJSON, err := buildIndexData(states, cfg.StaleMinutes)
	if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(cfg.OutputDirectory, indexDataFile), dataJSON); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(cfg.OutputDirectory, "index.html"), renderIndex(dataJSON))
}

func loadIndexState(path string) (map[string]nodeStatus, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]nodeStatus{}, nil
	}
	if err != nil {
		return nil, err
	}
	states := map[string]nodeStatus{}
	if err := json.Unmarshal(data, &states); err != nil {
		return nil, err
	}
	return states, nil
}

func saveIndexState(path string, states map[string]nodeStatus) error {
	data, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, data)
}

// systemStateMeta is the subset of a report's metrics needed to pull out
// io.systemd.Manager.SystemState without caring about the rest.
type systemStateMeta struct {
	Metrics []struct {
		Name  string          `json:"name"`
		Value json.RawMessage `json:"value"`
	} `json:"metrics"`
}

func extractSystemState(reportJSON []byte) string {
	var meta systemStateMeta
	if err := json.Unmarshal(reportJSON, &meta); err != nil {
		return ""
	}
	for _, m := range meta.Metrics {
		if m.Name != "io.systemd.Manager.SystemState" {
			continue
		}
		var state string
		if err := json.Unmarshal(m.Value, &state); err != nil {
			return ""
		}
		return state
	}
	return ""
}

func countMetrics(reportJSON []byte) int {
	var meta reportMeta
	if err := json.Unmarshal(reportJSON, &meta); err != nil {
		return 0
	}
	return len(meta.Metrics)
}

// nodeJSON is one host's entry in the JSON payload shared by index.html's
// embedded initial data and indexDataFile, the client polls to refresh it.
type nodeJSON struct {
	Hostname    string `json:"hostname"`
	LastSeenMs  int64  `json:"lastSeenMs"`
	SignStatus  string `json:"signStatus"`
	SystemState string `json:"systemState"`
	Metrics     int    `json:"metrics"`
}

// buildIndexData marshals the known hosts (sorted for a stable diff between
// runs), the configured staleness threshold and a generation timestamp into
// the JSON payload used both to seed index.html and to write indexDataFile.
func buildIndexData(states map[string]nodeStatus, staleMinutes int64) ([]byte, error) {
	hostnames := make([]string, 0, len(states))
	for h := range states {
		hostnames = append(hostnames, h)
	}
	sort.Strings(hostnames)

	nodes := make([]nodeJSON, 0, len(hostnames))
	for _, h := range hostnames {
		s := states[h]
		nodes = append(nodes, nodeJSON{
			Hostname:    s.Hostname,
			LastSeenMs:  s.LastSeen.UnixMilli(),
			SignStatus:  s.SignStatus,
			SystemState: s.SystemState,
			Metrics:     s.Metrics,
		})
	}

	data := struct {
		Nodes        []nodeJSON `json:"nodes"`
		StaleMinutes int64      `json:"staleMinutes"`
		GeneratedAt  string     `json:"generatedAt"`
	}{
		Nodes:        nodes,
		StaleMinutes: staleMinutes,
		GeneratedAt:  time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
	}

	out, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshaling index data: %w", err)
	}
	return out, nil
}

// renderIndex substitutes index_template.html's placeholder with dataJSON,
// the same payload written to indexDataFile, so the page has something to
// show before its first poll.
func renderIndex(dataJSON []byte) []byte {
	html := strings.Replace(string(indexTemplate), "__INDEX_DATA_JSON__", escapeScript(dataJSON), 1)
	return []byte(html)
}
