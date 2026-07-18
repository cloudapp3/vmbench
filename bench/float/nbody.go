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
	defaultNBodyCount = 1024
	defaultNBodySteps = 1000
)

type body struct {
	x, y, z    float64
	vx, vy, vz float64
	mass       float64
}

// NBodyWorkload benchmarks a deterministic n-body simulation.
type NBodyWorkload struct {
	bodies        []body
	steps         int
	dt            float64
	initialEnergy float64
	lastEnergy    float64
	lastDigest    uint64
}

// NewNBodyWorkload returns the default n-body workload.
func NewNBodyWorkload() bench.Workload {
	return newNBodyWorkload(defaultNBodyCount, defaultNBodySteps)
}

func newNBodyWorkload(count, steps int) *NBodyWorkload {
	bodies := make([]body, count)
	for idx := range bodies {
		angle := float64(idx) * 0.017
		bodies[idx] = body{
			x:    math.Cos(angle) * 100,
			y:    math.Sin(angle) * 100,
			z:    math.Sin(angle*0.7) * 50,
			vx:   math.Sin(angle*1.3) * 0.05,
			vy:   math.Cos(angle*0.9) * 0.05,
			vz:   math.Sin(angle*0.5) * 0.03,
			mass: 1 + float64((idx%17)+1)/20,
		}
	}
	return &NBodyWorkload{bodies: bodies, steps: steps, dt: 0.01, initialEnergy: totalEnergy(bodies)}
}

func (w *NBodyWorkload) Name() string { return "N-Body" }

func (w *NBodyWorkload) Category() string { return bench.CategoryFloat }

func (w *NBodyWorkload) Description() string {
	return "Simulates 1024 interacting bodies for 1000 steps with deterministic initial conditions."
}

func (*NBodyWorkload) ProcessedKind() bench.ProcessedKind { return bench.ProcessedOperations }

func (w *NBodyWorkload) Clone() bench.Workload {
	cp := *w
	cp.lastEnergy = 0
	cp.lastDigest = 0
	return &cp
}

func (w *NBodyWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	select {
	case <-ctx.Done():
		return 0, 0, ctx.Err()
	default:
	}
	bodies := append([]body(nil), w.bodies...)
	started := time.Now()
	for step := 0; step < w.steps; step++ {
		if step%8 == 0 {
			select {
			case <-ctx.Done():
				return 0, 0, ctx.Err()
			default:
			}
		}
		advanceBodies(bodies, w.dt)
	}
	w.lastEnergy = totalEnergy(bodies)
	w.lastDigest = digestBodies(bodies)
	common.ConsumeUint64(w.lastDigest)
	return time.Since(started), int64(len(bodies) * w.steps), nil
}

func (w *NBodyWorkload) Validate() error {
	if w.lastDigest == 0 || math.IsNaN(w.lastEnergy) || math.IsInf(w.lastEnergy, 0) {
		return errors.New("nbody produced invalid state")
	}
	if math.Abs(w.lastEnergy-w.initialEnergy) > math.Max(1, math.Abs(w.initialEnergy))*0.25 {
		return errors.New("nbody energy drift too large")
	}
	return nil
}

func (w *NBodyWorkload) Throughput(processed int64, elapsed time.Duration) (float64, string) {
	if processed <= 0 || elapsed <= 0 {
		return 0, "body-steps/s"
	}
	return float64(processed) / elapsed.Seconds(), "body-steps/s"
}

func advanceBodies(bodies []body, dt float64) {
	const softening = 0.01
	for i := range bodies {
		fx, fy, fz := 0.0, 0.0, 0.0
		for j := range bodies {
			if i == j {
				continue
			}
			dx := bodies[j].x - bodies[i].x
			dy := bodies[j].y - bodies[i].y
			dz := bodies[j].z - bodies[i].z
			dist2 := dx*dx + dy*dy + dz*dz + softening
			invDist := 1 / math.Sqrt(dist2)
			invDist3 := invDist * invDist * invDist
			force := bodies[j].mass * invDist3
			fx += dx * force
			fy += dy * force
			fz += dz * force
		}
		bodies[i].vx += fx * dt
		bodies[i].vy += fy * dt
		bodies[i].vz += fz * dt
	}
	for i := range bodies {
		bodies[i].x += bodies[i].vx * dt
		bodies[i].y += bodies[i].vy * dt
		bodies[i].z += bodies[i].vz * dt
	}
}

func totalEnergy(bodies []body) float64 {
	kinetic := 0.0
	potential := 0.0
	const softening = 0.01
	for i := range bodies {
		velocity2 := bodies[i].vx*bodies[i].vx + bodies[i].vy*bodies[i].vy + bodies[i].vz*bodies[i].vz
		kinetic += 0.5 * bodies[i].mass * velocity2
		for j := i + 1; j < len(bodies); j++ {
			dx := bodies[j].x - bodies[i].x
			dy := bodies[j].y - bodies[i].y
			dz := bodies[j].z - bodies[i].z
			distance := math.Sqrt(dx*dx + dy*dy + dz*dz + softening)
			potential -= (bodies[i].mass * bodies[j].mass) / distance
		}
	}
	return kinetic + potential
}

func digestBodies(bodies []body) uint64 {
	var digest uint64
	stride := len(bodies) / 32
	if stride == 0 {
		stride = 1
	}
	for idx := 0; idx < len(bodies); idx += stride {
		value := bodies[idx]
		digest ^= uint64((value.x+value.y+value.z)*1e6) + uint64(idx+1)
	}
	return digest
}
