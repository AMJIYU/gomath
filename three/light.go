package three

import (
	"fmt"
	"gomath/geom"
	"math"
	"slices"
)

// LightKind selects world-space sources used with ambient illumination.
// Its zero value preserves the original directional lighting behavior.
type LightKind uint8

const (
	// DirectionalLight is the default. Direction points surface-to-light.
	DirectionalLight LightKind = iota
	// PointLight uses one fixed world Position and per-sample distance/direction.
	// Point clouds remain unlit; point-light shadows are not implemented.
	PointLight
	// PointLightList uses Config.PointLights, adding Light.Ambient once.
	// All other legacy Light fields must be zero. No shadows are supported.
	PointLightList
)

// MaxLightAttenuation bounds PointLight's nonnegative inverse-world-unit-squared
// coefficient. Zero disables attenuation; coefficient k gives half strength at
// distance 1/sqrt(k). The regularized denominator is always at least one.
const MaxLightAttenuation = 1e12

const (
	MaxPointLights = 16
	// Light evaluations are charged per source per shaded internal sample,
	// across both mesh passes, including zero-strength sources/canceled normals.
	DefaultPointLightEvaluationLimit = 64 * 1024 * 1024
	MaxPointLightEvaluations         = 256 * 1024 * 1024
)

// PointLightSource is one colored world-space source in PointLightList mode.
// RGB channels and Diffuse must be finite in [0,1]. RGB has no alpha: source
// lighting never changes surface opacity. Channels multiply in sRGB, like the
// legacy shader. Position/Attenuation have the same bounds as PointLight.
// Zero RGB or Diffuse intentionally contributes no diffuse illumination.
type PointLightSource struct {
	Position             geom.Vec3
	RGB                  [3]float64
	Diffuse, Attenuation float64
}

func (l Light) validate(shadows bool) (geom.Vec3, error) {
	if !finite(l.Ambient) || !finite(l.Diffuse) || l.Ambient < 0 || l.Ambient > 1 || l.Diffuse < 0 || l.Diffuse > 1 {
		return geom.Vec3{}, fmt.Errorf("three: light ambient and diffuse must be finite in [0,1]")
	}
	switch l.Kind {
	case DirectionalLight:
		if !l.Direction.Finite() {
			return geom.Vec3{}, fmt.Errorf("three: directional light direction must be finite")
		}
		d, ok := unit(l.Direction)
		if l.Diffuse > 0 && !ok {
			return geom.Vec3{}, fmt.Errorf("three: diffuse light direction must be nonzero")
		}
		return d, nil
	case PointLight:
		if shadows {
			return geom.Vec3{}, fmt.Errorf("three: PointLight with Shadow.Enabled is unsupported; the shadow map requires DirectionalLight")
		}
		if !bounded(l.Position) || !finite(l.Attenuation) || l.Attenuation < 0 || l.Attenuation > MaxLightAttenuation {
			return geom.Vec3{}, fmt.Errorf("three: point light needs finite position components within +/- %g and attenuation in [0,%g]", MaxCoordinate, MaxLightAttenuation)
		}
		return geom.Vec3{}, nil
	case PointLightList:
		if shadows {
			return geom.Vec3{}, fmt.Errorf("three: PointLightList with Shadow.Enabled is unsupported; the shadow map requires DirectionalLight")
		}
		if l.Direction != (geom.Vec3{}) || l.Position != (geom.Vec3{}) || l.Diffuse != 0 || l.Attenuation != 0 {
			return geom.Vec3{}, fmt.Errorf("three: PointLightList cannot mix legacy Direction, Position, Diffuse or Attenuation; only Ambient is shared")
		}
		return geom.Vec3{}, nil
	default:
		return geom.Vec3{}, fmt.Errorf("three: light kind must be DirectionalLight, PointLight or PointLightList")
	}
}

// Validate the complete bounded list before copying; never retain an input
// backing array. Canonical order makes light permutations bit-identical even
// where differently ordered positive float sums would straddle a byte boundary.
func (c *Config) snapshotPointLights() error {
	if c.Light.Kind != PointLightList {
		if len(c.PointLights) != 0 || c.PointLightEvaluationLimit != 0 {
			return fmt.Errorf("three: PointLights and PointLightEvaluationLimit require PointLightList mode")
		}
		c.PointLights = nil
		return nil
	}
	if len(c.PointLights) > MaxPointLights {
		return fmt.Errorf("three: point light count exceeds %d", MaxPointLights)
	}
	if c.PointLightEvaluationLimit < 0 || c.PointLightEvaluationLimit > MaxPointLightEvaluations {
		return fmt.Errorf("three: point light evaluation limit must be 0 (default %d) or 1..%d", DefaultPointLightEvaluationLimit, MaxPointLightEvaluations)
	}
	for i, p := range c.PointLights {
		if _, err := (Light{Kind: PointLight, Position: p.Position, Diffuse: p.Diffuse, Attenuation: p.Attenuation}).validate(false); err != nil {
			return fmt.Errorf("three: point light %d: %w", i, err)
		}
		for _, channel := range p.RGB {
			if !finite(channel) || channel < 0 || channel > 1 {
				return fmt.Errorf("three: point light %d RGB must be finite in [0,1]", i)
			}
		}
	}
	if c.PointLightEvaluationLimit == 0 {
		c.PointLightEvaluationLimit = DefaultPointLightEvaluationLimit
	}
	c.PointLights = slices.Clone(c.PointLights)
	slices.SortFunc(c.PointLights, func(a, b PointLightSource) int {
		x := [8]float64{a.Position.X, a.Position.Y, a.Position.Z, a.RGB[0], a.RGB[1], a.RGB[2], a.Diffuse, a.Attenuation}
		y := [8]float64{b.Position.X, b.Position.Y, b.Position.Z, b.RGB[0], b.RGB[1], b.RGB[2], b.Diffuse, b.Attenuation}
		for i, v := range x {
			if v < y[i] {
				return -1
			}
			if v > y[i] {
				return 1
			}
		}
		return 0
	})
	return nil
}

// One local counter per render, never stored in the immutable Renderer.
type pointLightWork struct{ remaining int }

func (r *Renderer) pointIntensities(world, normal geom.Vec3, work *pointLightWork) ([3]float64, error) {
	ambient := r.config.Light.Ambient
	rgb := [3]float64{ambient, ambient, ambient}
	count := len(r.config.PointLights)
	if count > work.remaining {
		return rgb, fmt.Errorf("three: point light evaluation budget exceeded (limit %d, internal shaded samples times sources)", r.config.PointLightEvaluationLimit)
	}
	work.remaining -= count
	if !world.Finite() {
		return rgb, fmt.Errorf("three: interpolated point-light world position is nonfinite")
	}
	if normal == (geom.Vec3{}) {
		return rgb, nil
	}
	for _, p := range r.config.PointLights {
		if p.Diffuse == 0 {
			continue
		}
		v, err := (Light{Position: p.Position, Attenuation: p.Attenuation}).pointDiffuse(world, normal)
		if err != nil {
			return rgb, err
		}
		v *= p.Diffuse
		for i := range rgb {
			rgb[i] += p.RGB[i] * v
		}
	}
	for i := range rgb {
		rgb[i] = math.Min(1, rgb[i])
	}
	return rgb, nil
}

// The caller supplies a normalized world normal. Authored/model coordinates
// are bounded; Hypot and component division also support subnormal separations
// without overflowing a reciprocal. No mutable lighting state is retained.
func (l Light) pointDiffuse(world, normal geom.Vec3) (float64, error) {
	if !world.Finite() {
		return 0, fmt.Errorf("three: interpolated point-light world position is nonfinite")
	}
	v := l.Position.Sub(world)
	d := math.Hypot(math.Hypot(v.X, v.Y), v.Z)
	if !finite(d) {
		return 0, fmt.Errorf("three: point-light sample distance is nonfinite")
	}
	if d == 0 {
		return 0, nil
	}
	lambert := math.Max(0, normal.X*(v.X/d)+normal.Y*(v.Y/d)+normal.Z*(v.Z/d))
	return lambert / (1 + l.Attenuation*(d*d)), nil
}
