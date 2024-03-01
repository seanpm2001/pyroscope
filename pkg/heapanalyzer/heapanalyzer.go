package heapanalyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/grafana/dskit/services"
	"golang.org/x/exp/slices"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

type HeapAnalyzer struct {
	services.Service
	localDir string
	logger   log.Logger
}

func NewHeapAnalyzer(logger log.Logger) *HeapAnalyzer {
	h := &HeapAnalyzer{
		logger:   logger,
		localDir: "/tmp/heapdumps", // todo configure
	}
	h.Service = services.NewBasicService(nil, h.running, nil)
	err := os.MkdirAll(h.localDir, 0755)
	if err != nil {
		panic(err)
	}
	return h
}

func (h *HeapAnalyzer) running(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

const heapDumpInfoFile = "info.json"

// ingest file pyro.core.71993 and ___2pyroscope
// curl  -F core='@pyro.core.71993' -F exe='@___2pyroscope' -F labels='{"namespace":"foo", "pod":"bar"}' -X POST http://localhost:4040/heap-analyzer/ingest
func (h *HeapAnalyzer) IngestHandler(w http.ResponseWriter, r *http.Request) {
	id := uuid.New().String()
	level.Info(h.logger).Log("msg", "ingesting heap dump", "id", id)
	fr, err := r.MultipartReader()
	if err != nil {
		httputil.Error(w, err)
		return
	}
	heapDump := &HeapDump{
		Id:        id,
		CreatedAt: time.Now().UnixMilli(),
		Labels:    &typesv1.Labels{},
	}
	for {
		part, err := fr.NextPart()
		if err != nil {
			if err == io.EOF {
				break
			}
			httputil.Error(w, err)
			return
		}
		name := part.FormName()
		switch name {
		case "core", "exe":
			err = writeDumpFile(h.localDir, heapDump.Id, name, part)
			if err != nil {
				httputil.Error(w, err)
				return
			}
		case "labels":
			ls := map[string]string{}
			err = json.NewDecoder(part).Decode(&ls)
			if err != nil {
				httputil.Error(w, err)
				return
			}
			for k, v := range ls {
				heapDump.Labels.Labels = append(heapDump.Labels.Labels, &typesv1.LabelPair{Name: k, Value: v})
			}
			slices.SortFunc(heapDump.Labels.Labels, func(i, j *typesv1.LabelPair) int {
				return strings.Compare(i.Name, j.Name)
			})
		default:
			httputil.Error(w, fmt.Errorf("unknown part: %s", name))
			return
		}
	}
	heapDumpBytes, err := json.Marshal(heapDump)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	err = writeDumpFile(h.localDir, heapDump.Id, heapDumpInfoFile, bytes.NewReader(heapDumpBytes))
	if err != nil {
		httputil.Error(w, err)
		return
	}
}

// curl   http://localhost:4040/heap-analyzer/heap-dumps
func (h *HeapAnalyzer) HeapDumpsHandler(w http.ResponseWriter, r *http.Request) {
	var heapDumps []*HeapDump
	dumps, err := os.ReadDir(h.localDir)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	for _, d := range dumps {
		if !d.IsDir() {
			continue
		}
		id, err := uuid.Parse(d.Name())
		if err != nil {
			level.Error(h.logger).Log("msg", "error parsing heap dump id", "id", d.Name(), "err", err)
			continue
		}
		heapdump, err := h.readHeapDumpInfo(id.String())
		if err != nil {
			level.Error(h.logger).Log("msg", "error reading heap dump info", "id", d.Name(), "err", err)
			continue
		}
		heapDumps = append(heapDumps, heapdump)
	}
	data, err := json.Marshal(heapDumps)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	_, err = w.Write(data)
	if err != nil {
		httputil.Error(w, err)
	}
}

// curl   http://localhost:4040/heap-analyzer/heap-dump/0eed7d49-b9da-420d-b4a4-f041b2aca70b
func (h *HeapAnalyzer) HeapDumpHandler(w http.ResponseWriter, r *http.Request) {
	id := getHeapDumpId(r)
	_, err := uuid.Parse(id)
	if err != nil {
		httputil.Error(w, fmt.Errorf("invalid heap dump id: %s", id))
		return
	}
	level.Info(h.logger).Log("msg", "retrieving heap dump", "hid", id)
	info, err := h.readHeapDumpInfo(id)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	data, err := json.Marshal(info)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	_, err = w.Write(data)
	if err != nil {
		httputil.Error(w, err)
	}
}

func (h *HeapAnalyzer) HeapDumpObjectTypesHandler(w http.ResponseWriter, r *http.Request) {
	level.Info(h.logger).Log("msg", "retrieving heap dump object types", "hid", getHeapDumpId(r))
}

func (h *HeapAnalyzer) HeapDumpObjectsHandler(w http.ResponseWriter, r *http.Request) {
	level.Info(h.logger).Log("msg", "retrieving heap dump objects", "hid", getHeapDumpId(r))
}

func (h *HeapAnalyzer) HeapDumpObjectHandler(w http.ResponseWriter, r *http.Request) {
	level.Info(h.logger).Log("msg", "retrieving heap dump object", "hid", getHeapDumpId(r), "oid", getObjectId(r))
}

func (h *HeapAnalyzer) HeapDumpObjectReferencesHandler(w http.ResponseWriter, r *http.Request) {
	level.Info(h.logger).Log("msg", "retrieving heap dump object references", "hid", getHeapDumpId(r), "oid", getObjectId(r))
}

func (h *HeapAnalyzer) HeapDumpObjectFieldsHandler(w http.ResponseWriter, r *http.Request) {
	level.Info(h.logger).Log("msg", "retrieving heap dump object fields", "hid", getHeapDumpId(r), "oid", getObjectId(r))
}

func (h *HeapAnalyzer) readHeapDumpInfo(id string) (*HeapDump, error) {
	heapdump, err := os.ReadFile(h.localDir + "/" + id + "/" + heapDumpInfoFile)
	if err != nil {
		return nil, err
	}
	heapdumpBytes := new(HeapDump)
	err = json.Unmarshal(heapdump, heapdumpBytes)
	if err != nil {
		return nil, err
	}
	return heapdumpBytes, nil
}

func writeDumpFile(dir string, id string, name string, part io.Reader) error {
	fname := dir + "/" + id + "/" + name
	err := os.MkdirAll(filepath.Dir(fname), 0755)
	if err != nil {
		return err
	}
	f, err := os.Create(fname)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, part)
	if err != nil {
		return err
	}
	return nil
}

func getHeapDumpId(r *http.Request) string {
	vars := mux.Vars(r)
	return vars["id"]
}

func getObjectId(r *http.Request) string {
	vars := mux.Vars(r)
	return vars["oid"]
}

func getObjectFieldId(r *http.Request) string {
	vars := mux.Vars(r)
	return vars["fid"]
}