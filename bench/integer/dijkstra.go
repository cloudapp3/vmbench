package integer

import (
	"container/heap"
	"context"
	"errors"
	"math"
	"time"

	"github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/bench/common"
)

const (
	defaultGraphNodes  = 10_000
	defaultGraphDegree = 8
)

type graphEdge struct {
	to     int
	weight float64
}

// DijkstraWorkload benchmarks single-source shortest path over a deterministic sparse graph.
type DijkstraWorkload struct {
	graph         [][]graphEdge
	expectedCheck uint64
	lastCheck     uint64
}

// NewDijkstraWorkload returns the default Dijkstra workload.
func NewDijkstraWorkload() bench.Workload {
	return newDijkstraWorkload(defaultGraphNodes, defaultGraphDegree)
}

func newDijkstraWorkload(nodes, degree int) *DijkstraWorkload {
	graph := make([][]graphEdge, nodes)
	rng := common.NewRand()
	for from := 0; from < nodes; from++ {
		edges := make([]graphEdge, 0, degree)
		for step := 0; step < degree; step++ {
			to := rng.Intn(nodes)
			if to == from {
				to = (to + step + 1) % nodes
			}
			edges = append(edges, graphEdge{to: to, weight: 1 + float64(rng.Intn(1000))/10})
		}
		graph[from] = edges
	}
	workload := &DijkstraWorkload{graph: graph}
	workload.expectedCheck = checksumFloats(workload.shortestPaths(0))
	return workload
}

func (w *DijkstraWorkload) Name() string { return "Dijkstra" }

func (w *DijkstraWorkload) Category() string { return bench.CategoryInteger }

func (w *DijkstraWorkload) Description() string {
	return "Computes single-source shortest paths on a deterministic sparse graph with 10k nodes."
}

func (*DijkstraWorkload) ProcessedKind() bench.ProcessedKind { return bench.ProcessedOperations }

func (w *DijkstraWorkload) Clone() bench.Workload {
	cp := *w
	cp.lastCheck = 0
	return &cp
}

func (w *DijkstraWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	select {
	case <-ctx.Done():
		return 0, 0, ctx.Err()
	default:
	}
	started := time.Now()
	distances := w.shortestPaths(0)
	if ctx.Err() != nil {
		return 0, 0, ctx.Err()
	}
	w.lastCheck = checksumFloats(distances)
	common.ConsumeUint64(w.lastCheck)
	return time.Since(started), int64(len(distances)), nil
}

func (w *DijkstraWorkload) Validate() error {
	if w.lastCheck != w.expectedCheck {
		return errors.New("dijkstra validation failed")
	}
	return nil
}

func (w *DijkstraWorkload) Throughput(processed int64, elapsed time.Duration) (float64, string) {
	if processed <= 0 || elapsed <= 0 {
		return 0, "nodes/s"
	}
	return float64(processed) / elapsed.Seconds(), "nodes/s"
}

func (w *DijkstraWorkload) shortestPaths(source int) []float64 {
	dist := make([]float64, len(w.graph))
	for idx := range dist {
		dist[idx] = math.Inf(1)
	}
	dist[source] = 0
	pq := &priorityQueue{{node: source, distance: 0}}
	heap.Init(pq)
	for pq.Len() > 0 {
		item := heap.Pop(pq).(queueItem)
		if item.distance > dist[item.node] {
			continue
		}
		for _, edge := range w.graph[item.node] {
			next := item.distance + edge.weight
			if next >= dist[edge.to] {
				continue
			}
			dist[edge.to] = next
			heap.Push(pq, queueItem{node: edge.to, distance: next})
		}
	}
	return dist
}

type queueItem struct {
	node     int
	distance float64
}

type priorityQueue []queueItem

func (p priorityQueue) Len() int           { return len(p) }
func (p priorityQueue) Less(i, j int) bool { return p[i].distance < p[j].distance }
func (p priorityQueue) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p *priorityQueue) Push(x any)        { *p = append(*p, x.(queueItem)) }
func (p *priorityQueue) Pop() any {
	old := *p
	item := old[len(old)-1]
	*p = old[:len(old)-1]
	return item
}

func checksumFloats(values []float64) uint64 {
	var sum uint64
	stride := len(values) / 64
	if stride <= 0 {
		stride = 1
	}
	for idx := 0; idx < len(values); idx += stride {
		scaled := uint64(values[idx] * 1000)
		sum ^= scaled + uint64(idx+1)*0x517cc1b727220a95
	}
	return sum
}
