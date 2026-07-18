package float

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/bench/common"
)

const (
	defaultRTWidth  = 1280
	defaultRTHeight = 720
)

type vec3 struct{ x, y, z float64 }

type sphere struct {
	center  vec3
	radius  float64
	color   vec3
	reflect float64
}

type ray struct {
	origin vec3
	dir    vec3
}

// RayTracerWorkload benchmarks a small recursive ray tracer.
type RayTracerWorkload struct {
	width      int
	height     int
	maxDepth   int
	spheres    []sphere
	lastDigest uint64
	samples    [3]vec3
}

// NewRayTracerWorkload returns the default ray tracing workload.
func NewRayTracerWorkload() bench.Workload {
	return newRayTracerWorkload(defaultRTWidth, defaultRTHeight, 3)
}

func newRayTracerWorkload(width, height, depth int) *RayTracerWorkload {
	return &RayTracerWorkload{
		width:    width,
		height:   height,
		maxDepth: depth,
		spheres: []sphere{
			{center: vec3{0, 0.2, -3}, radius: 0.7, color: vec3{0.9, 0.2, 0.2}, reflect: 0.3},
			{center: vec3{-1.2, 0.1, -4}, radius: 0.8, color: vec3{0.2, 0.8, 0.3}, reflect: 0.4},
			{center: vec3{1.3, -0.1, -4.2}, radius: 0.9, color: vec3{0.2, 0.4, 0.9}, reflect: 0.5},
		},
	}
}

func (w *RayTracerWorkload) Name() string { return "Ray Trace" }

func (w *RayTracerWorkload) Category() string { return bench.CategoryFloat }

func (w *RayTracerWorkload) Description() string {
	return "Renders a 1280x720 scene with spheres, a ground plane, and up to 3 reflection bounces."
}

func (*RayTracerWorkload) ProcessedKind() bench.ProcessedKind { return bench.ProcessedOperations }

func (w *RayTracerWorkload) Clone() bench.Workload {
	cp := *w
	cp.lastDigest = 0
	cp.samples = [3]vec3{}
	return &cp
}

func (w *RayTracerWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	started := time.Now()
	var digest uint64
	checkpoints := [][2]int{{0, 0}, {w.width / 2, w.height / 2}, {w.width - 1, w.height - 1}}
	camera := vec3{0, 0.1, 0}
	index := 0
	for y := 0; y < w.height; y++ {
		if y%8 == 0 {
			select {
			case <-ctx.Done():
				return 0, 0, ctx.Err()
			default:
			}
		}
		for x := 0; x < w.width; x++ {
			uvx := (2*(float64(x)+0.5)/float64(w.width) - 1) * float64(w.width) / float64(w.height)
			uvy := 1 - 2*(float64(y)+0.5)/float64(w.height)
			color := w.trace(ray{origin: camera, dir: unitVector(vec3{uvx, uvy, -1})}, 0)
			digest ^= uint64((color.x+color.y+color.z)*1e6) + uint64(x+1)*1099511628211 + uint64(y+1)
			for sampleIdx, point := range checkpoints {
				if x == point[0] && y == point[1] {
					w.samples[sampleIdx] = color
					index++
				}
			}
		}
	}
	w.lastDigest = digest
	common.ConsumeUint64(digest)
	return time.Since(started), int64(w.width * w.height), nil
}

func (w *RayTracerWorkload) Validate() error {
	if w.lastDigest == 0 {
		return errors.New("ray trace digest is zero")
	}
	points := [][2]int{{0, 0}, {w.width / 2, w.height / 2}, {w.width - 1, w.height - 1}}
	camera := vec3{0, 0.1, 0}
	for idx, point := range points {
		uvx := (2*(float64(point[0])+0.5)/float64(w.width) - 1) * float64(w.width) / float64(w.height)
		uvy := 1 - 2*(float64(point[1])+0.5)/float64(w.height)
		color := w.trace(ray{origin: camera, dir: unitVector(vec3{uvx, uvy, -1})}, 0)
		if distanceVec(color, w.samples[idx]) > 1e-6 {
			return errors.New("ray trace sample mismatch")
		}
	}
	return nil
}

func (w *RayTracerWorkload) Throughput(processed int64, elapsed time.Duration) (float64, string) {
	if processed <= 0 || elapsed <= 0 {
		return 0, "pixels/s"
	}
	return float64(processed) / elapsed.Seconds(), "pixels/s"
}

func (w *RayTracerWorkload) trace(r ray, depth int) vec3 {
	if depth >= w.maxDepth {
		return vec3{}
	}
	hitColor, hitPoint, normal, reflectivity, hit := w.intersect(r)
	if !hit {
		return vec3{0.2, 0.4, 0.8}
	}
	lightDir := unitVector(vec3{0.7, 1, -0.5})
	diffuse := math.Max(0, dot(normal, lightDir))
	color := scale(hitColor, 0.1+0.9*diffuse)
	if reflectivity <= 0 {
		return color
	}
	reflectedDir := unitVector(sub(r.dir, scale(normal, 2*dot(r.dir, normal))))
	reflected := w.trace(ray{origin: add(hitPoint, scale(normal, 0.001)), dir: reflectedDir}, depth+1)
	return add(scale(color, 1-reflectivity), scale(reflected, reflectivity))
}

func (w *RayTracerWorkload) intersect(r ray) (vec3, vec3, vec3, float64, bool) {
	closestT := math.MaxFloat64
	var hitColor, hitPoint, normal vec3
	reflectivity := 0.0
	hit := false
	for _, object := range w.spheres {
		oc := sub(r.origin, object.center)
		a := dot(r.dir, r.dir)
		b := 2 * dot(oc, r.dir)
		c := dot(oc, oc) - object.radius*object.radius
		discriminant := b*b - 4*a*c
		if discriminant < 0 {
			continue
		}
		t := (-b - math.Sqrt(discriminant)) / (2 * a)
		if t <= 0.001 || t >= closestT {
			continue
		}
		closestT = t
		hit = true
		hitPoint = add(r.origin, scale(r.dir, t))
		normal = unitVector(sub(hitPoint, object.center))
		hitColor = object.color
		reflectivity = object.reflect
	}
	planeT := -(r.origin.y + 1.0) / r.dir.y
	if planeT > 0.001 && planeT < closestT {
		hit = true
		hitPoint = add(r.origin, scale(r.dir, planeT))
		normal = vec3{0, 1, 0}
		checker := (int(math.Floor(hitPoint.x))+int(math.Floor(hitPoint.z)))&1 == 0
		if checker {
			hitColor = vec3{0.9, 0.9, 0.9}
		} else {
			hitColor = vec3{0.3, 0.3, 0.3}
		}
		reflectivity = 0.1
	}
	return hitColor, hitPoint, normal, reflectivity, hit
}

func add(a, b vec3) vec3           { return vec3{a.x + b.x, a.y + b.y, a.z + b.z} }
func sub(a, b vec3) vec3           { return vec3{a.x - b.x, a.y - b.y, a.z - b.z} }
func scale(v vec3, s float64) vec3 { return vec3{v.x * s, v.y * s, v.z * s} }
func dot(a, b vec3) float64        { return a.x*b.x + a.y*b.y + a.z*b.z }
func unitVector(v vec3) vec3 {
	length := math.Sqrt(dot(v, v))
	if length == 0 {
		return v
	}
	return scale(v, 1/length)
}
func distanceVec(a, b vec3) float64 {
	return math.Sqrt((a.x-b.x)*(a.x-b.x) + (a.y-b.y)*(a.y-b.y) + (a.z-b.z)*(a.z-b.z))
}
