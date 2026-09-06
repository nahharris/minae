package mesh

import (
	"math"
	"sync"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/blocks/model"
	"github.com/nahharris/minae/internal/gfx/atlas"
	"github.com/nahharris/minae/internal/platform/config"
)

type meshBuilder struct {
	vertices  []float32
	texcoords []float32
	normals   []float32
	colors    []uint8
}

func (b *meshBuilder) reset() {
	b.vertices = b.vertices[:0]
	b.texcoords = b.texcoords[:0]
	b.normals = b.normals[:0]
	b.colors = b.colors[:0]
}

func (b *meshBuilder) release() {
	b.reset()
	meshBuilderPool.Put(b)
}

var meshBuilderPool = sync.Pool{
	New: func() any {
		return &meshBuilder{}
	},
}

// Pre-computed face normals to avoid allocations.
var (
	normalTop    = rl.Vector3{X: 0, Y: 1, Z: 0}
	normalBottom = rl.Vector3{X: 0, Y: -1, Z: 0}
	normalLeft   = rl.Vector3{X: -1, Y: 0, Z: 0}
	normalRight  = rl.Vector3{X: 1, Y: 0, Z: 0}
	normalFront  = rl.Vector3{X: 0, Y: 0, Z: 1}
	normalBack   = rl.Vector3{X: 0, Y: 0, Z: -1}
)

func buildChunkMesh(chunk ChunkReader, world WorldReader, uvLookup UVLookup) *meshBuilder {
	builder := meshBuilderPool.Get().(*meshBuilder)
	builder.reset()

	// addQuad emits one quad (two triangles) for the block at local (x,y,z)
	// (global (gx,gy,gz)) covering face q.Face. Vertex colour packs per-vertex
	// lighting inputs instead of true colour: R = skylight (0..15 mapped onto
	// 0..255, exactly light*17), G = block light (same mapping, for glowstone
	// and other emitters), B = ambient occlusion, A = opacity, always fully
	// opaque. Alpha is deliberately never used to carry light: raylib blends
	// by default, so a dark-but-alpha'd vertex used to render as see-through
	// sky instead of dark. Per-face brightness (e.g. top brighter than sides)
	// is applied in the shader, not baked in here, so it stays tunable
	// without re-meshing.
	//
	// Light and AO are sampled per corner rather than once for the whole
	// quad, so a single face can carry a gradient. Positions, texture
	// coordinates and colours are computed into per-corner arrays first and
	// only then emitted, in one order decided once, so a triangulation flip
	// (done to avoid the classic voxel AO crease) can never desynchronise a
	// vertex's position from another vertex's UV or colour.
	addQuad := func(x, y, z, gx, gy, gz int, q model.Quad, uv atlas.UV) {
		fx, fy, fz := float32(x), float32(y), float32(z)

		n := normalForFace(q.Face)

		positions := [4]model.Vec3{q.V1, q.V2, q.V3, q.V4}

		var center model.Vec3
		for _, p := range positions {
			center.X += p.X
			center.Y += p.Y
			center.Z += p.Z
		}
		center.X /= 4
		center.Y /= 4
		center.Z /= 4

		var uvs [4][2]float32
		var cols [4][4]uint8
		var aoLevels [4]int
		for i, p := range positions {
			u, v := uvForVertex(q.Face, p, uv)
			uvs[i] = [2]float32{u, v}

			sky, block, ao, level := cornerLightAndAO(world, gx, gy, gz, q.Face, p, center)
			cols[i] = [4]uint8{sky, block, ao, 255}
			aoLevels[i] = level
		}

		// Default triangulation is (V1,V2,V3),(V1,V3,V4), splitting along the
		// V1-V3 diagonal. Flip to (V1,V2,V4),(V2,V3,V4) — the V2-V4 diagonal —
		// when that is the darker of the two.
		//
		// The quad must split along the diagonal through its *darker* corners.
		// Light is interpolated across each triangle independently, so a split
		// along the brighter diagonal can leave one triangle with every corner
		// unoccluded. That triangle then renders at full brightness right up to
		// the edge it shares with the shaded half: a hard bright wedge where a
		// soft corner shadow belongs. Splitting through the occluded corner
		// puts it in both triangles instead, and the shadow radiates from it.
		//
		// This compares raw occlusion levels, not the ramped B-channel bytes.
		// The two agree only while aoRamp is evenly spaced, and aoRamp is
		// explicitly documented as free to tune — including non-linearly.
		// Comparing the bytes would make an uneven ramp silently change which
		// diagonal a quad splits along.
		order := [6]int{0, 1, 2, 0, 2, 3}
		if aoLevels[0]+aoLevels[2] > aoLevels[1]+aoLevels[3] {
			order = [6]int{0, 1, 3, 1, 2, 3}
		}

		for _, idx := range order {
			p := positions[idx]
			builder.vertices = append(builder.vertices, fx+p.X, fy+p.Y, fz+p.Z)
			builder.normals = append(builder.normals, n.X, n.Y, n.Z)
			builder.texcoords = append(builder.texcoords, uvs[idx][0], uvs[idx][1])
			c := cols[idx]
			builder.colors = append(builder.colors, c[0], c[1], c[2], c[3])
		}
	}

	quads := make([]model.Quad, 0, 16)

	for x := range config.ChunkWidth {
		for y := range config.ChunkHeight {
			for z := range config.ChunkWidth {
				block, meta := chunk.GetBlockState(x, y, z)
				if block == nil {
					continue
				}

				blockModel := block.Model
				if blockModel == nil {
					blockModel = model.CompileModel(block.ID, block.ModelSpec)
				}

				gx, gy, gz := chunk.ChunkX()*config.ChunkWidth+x, y, chunk.ChunkZ()*config.ChunkWidth+z

				quads = blockModel.AppendQuads(quads[:0], meta)
				for _, q := range quads {
					if q.Cull {
						dx, dy, dz := offsetForFace(q.Face)
						neighbor, nmeta := world.GetBlockState(gx+dx, gy+dy, gz+dz)
						if neighbor != nil {
							neighborModel := neighbor.Model
							if neighborModel == nil {
								neighborModel = model.CompileModel(neighbor.ID, neighbor.ModelSpec)
							}

							region := quadRegion(q)
							if neighborModel.Occludes(nmeta, q.Face.Opposite(), region) {
								continue
							}
						}
					}

					uv := atlas.UV{U0: 0, V0: 0, U1: 1, V1: 1}
					if uvLookup != nil && q.Texture != "" {
						if r, ok := uvLookup.UV(q.Texture); ok {
							uv = r
						}
					}

					addQuad(x, y, z, gx, gy, gz, q, uv)
				}
			}
		}
	}

	if len(builder.vertices) == 0 {
		builder.release()
		return nil
	}

	return builder
}

func normalForFace(face model.Face) rl.Vector3 {
	switch face {
	case model.FaceRight:
		return normalRight
	case model.FaceLeft:
		return normalLeft
	case model.FaceTop:
		return normalTop
	case model.FaceBottom:
		return normalBottom
	case model.FaceFront:
		return normalFront
	default:
		return normalBack
	}
}

func offsetForFace(face model.Face) (dx, dy, dz int) {
	switch face {
	case model.FaceRight:
		return 1, 0, 0
	case model.FaceLeft:
		return -1, 0, 0
	case model.FaceTop:
		return 0, 1, 0
	case model.FaceBottom:
		return 0, -1, 0
	case model.FaceFront:
		return 0, 0, 1
	default:
		return 0, 0, -1
	}
}

// aoRamp maps an ambient-occlusion level (0 = most occluded corner, 3 =
// fully unoccluded) onto the mesh's B channel. The levels sit at 0.6, 0.73,
// 0.87 and 1.0 of full brightness; tune these four values freely if the AO
// contrast needs adjusting, including non-linearly.
//
// Nothing else depends on their spacing, and that is deliberate: the
// triangulation flip compares raw occlusion levels rather than these bytes,
// so an uneven ramp cannot quietly change which diagonal a quad splits along.
var aoRamp = [4]uint8{153, 187, 221, 255}

// cellSample holds the per-cell state the smooth-lighting and ambient-
// occlusion sampling needs: the two light channels and whether the cell lets
// light through (see sampleCell for what "transparent" means at chunk
// boundaries).
type cellSample struct {
	sky, block  uint8
	transparent bool

	// occludes is deliberately separate from transparent: an emitting block is
	// solid (light does not pass through it) but casts no ambient occlusion.
	occludes bool
}

// sampleCell reads the light and solidity of one global cell for the
// purposes of smooth lighting and AO.
//
// The light engine treats an unloaded chunk as opaque (skylight 0) so that
// light never leaks in from nowhere; that is correct for the engine, but at
// the edge of the loaded world it would render the outward-facing faces as a
// black wall. That is a cosmetic problem for the renderer, not the engine,
// so an unloaded chunk substitutes full-bright skylight here instead of
// changing what the engine reports. Block light gets no such fallback: there
// is no reason to pretend an unloaded chunk contains a torch, so it stays 0.
// A cell in an unloaded chunk is also treated as transparent, matching that
// same "pretend it's open space" fallback rather than "pretend it's solid
// rock", and consistent with GetBlockState already reporting air (nil) for
// any position in a chunk that isn't loaded.
func sampleCell(world WorldReader, x, y, z int) cellSample {
	block, _ := world.GetBlockState(x, y, z)
	if !world.HasChunkAt(x, z) {
		return cellSample{sky: 15, block: 0, transparent: block == nil, occludes: false}
	}
	return cellSample{
		sky:         world.GetSkyLight(x, y, z),
		block:       world.GetBlockLight(x, y, z),
		transparent: block == nil,
		occludes:    occludes(block),
	}
}

// occludes reports whether a block casts ambient occlusion on its neighbours.
//
// This is deliberately not the same question as transparency. Ambient
// occlusion approximates how much surrounding geometry blocks incoming light,
// so a light-emitting block is excluded: a glowstone is not blocking light, it
// is light. Letting one occlude produces a dark halo on exactly the surfaces it
// is illuminating, which reads as the lamp casting its own shadow.
//
// Transparency still governs which cells contribute to the smooth-lighting
// average, and an emitter is solid there — light does not pass through it.
func occludes(b *blocks.Block) bool {
	return b != nil && b.LightLevel == 0
}

// faceAxes returns the two local-space axis indices (0=X, 1=Y, 2=Z) tangent
// to the given face, i.e. the two axes perpendicular to the face's own
// normal axis. It depends only on which axis is normal, so a face and its
// opposite (e.g. top and bottom) return the same pair.
func faceAxes(face model.Face) (axisU, axisW int) {
	switch face {
	case model.FaceRight, model.FaceLeft:
		return 1, 2 // Y, Z
	case model.FaceTop, model.FaceBottom:
		return 0, 2 // X, Z
	default: // FaceFront, FaceBack
		return 0, 1 // X, Y
	}
}

// vecComponent returns the given axis (0=X, 1=Y, 2=Z) component of v.
func vecComponent(v model.Vec3, axis int) float32 {
	switch axis {
	case 0:
		return v.X
	case 1:
		return v.Y
	default:
		return v.Z
	}
}

// signOffset reports the sign of v-c as -1, 0 or 1. It returns 0 exactly
// when v == c, which callers use to detect a degenerate quad corner (one
// that does not lie strictly to one side of the quad's centre).
func signOffset(v, c float32) int {
	switch {
	case v > c:
		return 1
	case v < c:
		return -1
	default:
		return 0
	}
}

// axisOffset turns a sign (-1, 0 or 1) along the given axis (0=X, 1=Y, 2=Z)
// into an integer cell offset.
func axisOffset(axis, sign int) (dx, dy, dz int) {
	switch axis {
	case 0:
		return sign, 0, 0
	case 1:
		return 0, sign, 0
	default:
		return 0, 0, sign
	}
}

// averageChannel averages one light channel (chosen by get) over whichever
// of the four cells are transparent, in 0..255 space, so precision is not
// lost by averaging 0..15 levels first. Opaque cells are excluded rather
// than counted as zero. If none of the four are transparent, it falls back
// to the value at a (the cell the face looks into), even though a is itself
// excluded from the ambient-occlusion neighbourhood below.
func averageChannel(a, b, c, d cellSample, get func(cellSample) uint8) uint8 {
	sum, count := 0, 0
	for _, s := range [4]cellSample{a, b, c, d} {
		if !s.transparent {
			continue
		}
		sum += int(get(s)) * 17
		count++
	}
	if count == 0 {
		return get(a) * 17
	}
	return uint8(sum / count)
}

// boolToInt converts a bool to 0 or 1, for the ambient-occlusion formula
// below which is stated in those terms.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// cornerLightAndAO computes the smoothed skylight and block light, and the
// ambient-occlusion byte, for one corner of a quad on the given face. The
// quad's parent block sits at global cell (gx,gy,gz); corner is that
// corner's position and center the quad's centroid, both in the block's
// local (0..1) space.
//
// The four cells touching the corner on the outward side are:
//
//	a = p + n          (the cell the face looks into)
//	b = p + n + u
//	c = p + n + w
//	d = p + n + u + w
//
// where n is the face's outward offset and u, w are unit offsets along the
// face's two tangent axes, pointing from the quad's centre toward this
// corner. Light is the average of a, b, c and d (excluding opaque cells);
// ambient occlusion uses only b, c and d, per the classic voxel AO rule:
// fully occluded when both b and c are solid, otherwise 3 minus the number
// of solid cells among b, c and d.
//
// Quads are model-driven and need not lie on the cube boundary (a slab's top
// face sits at y=0.5); such faces still sample as if they were the cube's
// own face, per the existing offsetForFace convention.
//
// If a corner is degenerate -- its position does not lie strictly to one
// side of the centre along one of the tangent axes -- this falls back to the
// pre-smoothing behaviour for that corner: no occlusion, light sampled from
// a alone, so a zero tangent offset can never produce a divide-by-zero or a
// nonsense neighbour cell.
func cornerLightAndAO(world WorldReader, gx, gy, gz int, face model.Face, corner, center model.Vec3) (skylightByte, blockLightByte, aoByte uint8, aoLevel int) {
	nx, ny, nz := offsetForFace(face)
	axisU, axisW := faceAxes(face)

	su := signOffset(vecComponent(corner, axisU), vecComponent(center, axisU))
	sw := signOffset(vecComponent(corner, axisW), vecComponent(center, axisW))

	a := sampleCell(world, gx+nx, gy+ny, gz+nz)

	if su == 0 || sw == 0 {
		return a.sky * 17, a.block * 17, aoRamp[3], 3
	}

	ux, uy, uz := axisOffset(axisU, su)
	wx, wy, wz := axisOffset(axisW, sw)

	b := sampleCell(world, gx+nx+ux, gy+ny+uy, gz+nz+uz)
	c := sampleCell(world, gx+nx+wx, gy+ny+wy, gz+nz+wz)
	d := sampleCell(world, gx+nx+ux+wx, gy+ny+uy+wy, gz+nz+uz+wz)

	skylightByte = averageChannel(a, b, c, d, func(s cellSample) uint8 { return s.sky })
	blockLightByte = averageChannel(a, b, c, d, func(s cellSample) uint8 { return s.block })

	// Occlusion is asked of the block's shape at the corner, not of the cell's
	// occupancy. The probe point is the shared corner nudged a quarter cell into
	// each neighbour, so a slab only shades the half of its cell it actually
	// fills.
	vx := float32(gx) + corner.X
	vy := float32(gy) + corner.Y
	vz := float32(gz) + corner.Z

	var scratch []model.Box
	probe := func(dx, dy, dz int) bool {
		return occludesCorner(world,
			gx+dx, gy+dy, gz+dz,
			vx+cornerProbe*float32(dx), vy+cornerProbe*float32(dy), vz+cornerProbe*float32(dz),
			&scratch)
	}

	side1Solid := probe(nx+ux, ny+uy, nz+uz)
	side2Solid := probe(nx+wx, ny+wy, nz+wz)
	cornerSolid := probe(nx+ux+wx, ny+uy+wy, nz+uz+wz)

	level := 3 - boolToInt(side1Solid) - boolToInt(side2Solid) - boolToInt(cornerSolid)
	if side1Solid && side2Solid {
		// Where two neighbours meet along an edge, the diagonal cell touches
		// this vertex at a single point and contributes no solid angle, so the
		// corner is fully occluded whatever the diagonal holds.
		level = 0
	}

	return skylightByte, blockLightByte, aoRamp[level], level
}

func quadRegion(q model.Quad) model.Rect {
	minU, minV := float32(math.Inf(1)), float32(math.Inf(1))
	maxU, maxV := float32(math.Inf(-1)), float32(math.Inf(-1))

	vs := [4]model.Vec3{q.V1, q.V2, q.V3, q.V4}
	for _, v := range vs {
		var u, w float32
		switch q.Face {
		case model.FaceRight, model.FaceLeft:
			u, w = v.Z, v.Y
		case model.FaceFront, model.FaceBack:
			u, w = v.X, v.Y
		default: // Top/Bottom
			u, w = v.X, v.Z
		}

		if u < minU {
			minU = u
		}
		if w < minV {
			minV = w
		}
		if u > maxU {
			maxU = u
		}
		if w > maxV {
			maxV = w
		}
	}

	if math.IsInf(float64(minU), 1) || math.IsInf(float64(minV), 1) {
		return model.Rect{}
	}

	return model.Rect{MinU: minU, MinV: minV, MaxU: maxU, MaxV: maxV}
}

func uvForVertex(face model.Face, v model.Vec3, r atlas.UV) (u, vt float32) {
	localU, localV := float32(0), float32(0)

	// Convention:
	// - Texture origin is top-left (V increases downward).
	// - On vertical faces, V=0 maps to world-up (Y=1), V=1 maps to world-down (Y=0).
	switch face {
	case model.FaceFront: // +Z
		localU = 1 - v.X
		localV = 1 - v.Y
	case model.FaceBack: // -Z
		localU = v.X
		localV = 1 - v.Y
	case model.FaceRight: // +X
		localU = v.Z
		localV = 1 - v.Y
	case model.FaceLeft: // -X
		localU = 1 - v.Z
		localV = 1 - v.Y
	case model.FaceTop: // +Y
		localU = v.X
		localV = v.Z
	default: // model.FaceBottom (-Y)
		localU = v.X
		localV = 1 - v.Z
	}

	// Clamp to avoid tiny floating errors from rotations.
	if localU < 0 {
		localU = 0
	} else if localU > 1 {
		localU = 1
	}
	if localV < 0 {
		localV = 0
	} else if localV > 1 {
		localV = 1
	}

	u = r.U0 + (r.U1-r.U0)*localU
	vt = r.V0 + (r.V1-r.V0)*localV
	return u, vt
}

// cornerProbe is how far into a neighbouring cell the occlusion test samples,
// measured from the shared corner.
//
// A quarter of a cell resolves half-block shapes unambiguously: probing from a
// corner on a cell's lower face lands at 0.25, inside a bottom slab and clear
// of a top one, and probing from the upper face lands at 0.75, the reverse.
// Shapes with features finer than a half cell — stairs at quarter steps, say —
// would need a finer probe or genuine area sampling, and this constant is where
// that would be revisited.
const cornerProbe = 0.25

// occludesCorner reports whether the block at the given cell has solid geometry
// at the given point, expressed in world space.
//
// This asks about the block's *shape*, not merely whether the cell is occupied.
// A slab fills half its cell, so whether it occludes depends on which half the
// corner sits against: a top slab beside a floor does not shade that floor,
// because the two never touch. Testing occupancy alone made every slab cast a
// full block's worth of occlusion.
//
// A light-emitting block never occludes: it is not blocking light, it is light.
func occludesCorner(world WorldReader, cx, cy, cz int, px, py, pz float32, scratch *[]model.Box) bool {
	block, meta := world.GetBlockState(cx, cy, cz)
	if block == nil || block.LightLevel > 0 {
		return false
	}

	blockModel := block.Model
	if blockModel == nil {
		blockModel = model.CompileModel(block.ID, block.ModelSpec)
	}
	*scratch = blockModel.CollisionBoxes((*scratch)[:0], meta)

	lx, ly, lz := px-float32(cx), py-float32(cy), pz-float32(cz)
	for _, b := range *scratch {
		if lx >= b.Min.X && lx <= b.Max.X &&
			ly >= b.Min.Y && ly <= b.Max.Y &&
			lz >= b.Min.Z && lz <= b.Max.Z {
			return true
		}
	}
	return false
}
