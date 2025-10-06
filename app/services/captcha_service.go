// Package services provides external service integrations and technical concerns like notifications and tokens
package services

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"math"
	"math/rand"
	"sync"
	"time"

	crand "crypto/rand"
	"encoding/binary"

	"github.com/google/uuid"
	"github.com/wenlng/go-captcha/v2/rotate"
)

// CaptchaService exposes methods to generate and verify captchas
// This implementation uses the rotate captcha mode from go-captcha
// Reference: https://github.com/wenlng/go-captcha
//
// Flow:
// - Generate: returns a challenge ID and two base64 images (master and thumb)
// - Verify: validates a user-provided angle against the stored target angle with tolerance
// - Challenges are stored in-memory with TTL and removed on success/expiry
//
// Note: The frontend should render the images and capture the rotation angle that the user applies.
// On submit, send the angle along with the challenge ID for verification.
type CaptchaService interface {
	// GenerateRotate creates a rotate captcha challenge and returns the assets and challenge ID
	GenerateRotate(ctx context.Context) (*RotateChallenge, error)
	// VerifyRotate verifies the provided user angle for a given challenge ID
	VerifyRotate(ctx context.Context, challengeID string, userAngle float64) bool
}

type RotateChallenge struct {
	ID                string
	MasterImageBase64 string
	ThumbImageBase64  string
}

type captchaServiceImpl struct {
	rotator   rotate.Captcha
	store     *memoryStore
	padding   int // tolerance for angle validation
	imgSizePx int // square size for rotate captcha images
}

// NewCaptchaServiceRotate constructs a CaptchaService using rotate mode
// ttl: time window during which a challenge remains valid
// padding: acceptable angle difference (degrees) when validating
// imgSizePx: square size for generated images (e.g., 220)
func NewCaptchaServiceRotate(ttl time.Duration, padding int, imgSizePx int) (CaptchaService, error) {
	if imgSizePx <= 0 {
		imgSizePx = 220
	}

	// Build a rotator with a few programmatically generated background images
	builder := rotate.NewBuilder(
		rotate.WithImageSquareSize(imgSizePx),
	)
	builder.SetResources(
		rotate.WithImages(generateRotateBackgrounds(8, imgSizePx)),
	)
	rotator := builder.Make()

	return &captchaServiceImpl{
		rotator:   rotator,
		store:     newMemoryStore(ttl),
		padding:   padding,
		imgSizePx: imgSizePx,
	}, nil
}

func (s *captchaServiceImpl) GenerateRotate(ctx context.Context) (*RotateChallenge, error) {
	captData, err := s.rotator.Generate()
	if err != nil {
		return nil, err
	}

	block := captData.GetData()
	if block == nil {
		return nil, err
	}

	masterB64, err := captData.GetMasterImage().ToBase64()
	if err != nil {
		return nil, err
	}
	thumbB64, err := captData.GetThumbImage().ToBase64()
	if err != nil {
		return nil, err
	}

	challengeID := uuid.New().String()
	// Store target angle with TTL
	s.store.Set(challengeID, storeEntry{
		targetAngle: block.Angle,
		expiresAt:   time.Now().Add(s.store.ttl),
	})

	return &RotateChallenge{
		ID:                challengeID,
		MasterImageBase64: masterB64,
		ThumbImageBase64:  thumbB64,
	}, nil
}

func (s *captchaServiceImpl) VerifyRotate(ctx context.Context, challengeID string, userAngle float64) bool {
	entry, ok := s.store.Get(challengeID)
	if !ok {
		return false
	}

	// Round user-provided angle to integer degrees expected by validator
	ua := int(math.Round(userAngle))
	ok = rotate.Validate(ua, entry.targetAngle, s.padding)
	// consume on success or failure
	s.store.Delete(challengeID)

	return ok
}

// --- In-memory store with TTL ---

type storeEntry struct {
	targetAngle int
	expiresAt   time.Time
}

type memoryStore struct {
	mu  sync.RWMutex
	m   map[string]storeEntry
	ttl time.Duration
}

func newMemoryStore(ttl time.Duration) *memoryStore {
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	ms := &memoryStore{
		m:   make(map[string]storeEntry),
		ttl: ttl,
	}
	// Background cleanup goroutine
	go ms.cleanupLoop()
	return ms
}

func (s *memoryStore) Set(id string, e storeEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[id] = e
}

func (s *memoryStore) Get(id string) (storeEntry, bool) {
	s.mu.RLock()
	e, ok := s.m[id]
	s.mu.RUnlock()
	if !ok {
		return storeEntry{}, false
	}
	if time.Now().After(e.expiresAt) {
		// expired
		s.Delete(id)
		return storeEntry{}, false
	}
	return e, true
}

func (s *memoryStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, id)
}

func (s *memoryStore) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		s.mu.Lock()
		for k, v := range s.m {
			if now.After(v.expiresAt) {
				delete(s.m, k)
			}
		}
		s.mu.Unlock()
	}
}

// --- Utility: generate simple background images programmatically ---

func generateRotateBackgrounds(n int, size int) []image.Image {
	if n <= 0 {
		n = 1
	}
	imgs := make([]image.Image, 0, n)
	// Create a crypto-seeded PRNG for visual variety
	seed := cryptoSeed() ^ time.Now().UnixNano()
	r := rand.New(rand.NewSource(seed))
	for i := 0; i < n; i++ {
		imgs = append(imgs, newNoiseGradientImageWithRand(size, size, r))
	}
	return imgs
}

func newNoiseGradientImageWithRand(w, h int, r *rand.Rand) image.Image {
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	// Gradient background with per-pixel noise
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// simple radial gradient + noise
			dx := float64(x - w/2)
			dy := float64(y - h/2)
			dist := math.Sqrt(dx*dx + dy*dy)
			t := dist / float64(w/2)
			if t > 1 {
				t = 1
			}
			base := uint8(200 - int(150*t))
			noise := uint8(r.Intn(30))
			rgba.Set(x, y, color.RGBA{R: base + noise/3, G: base, B: 255 - base/2, A: 255})
		}
	}
	// Overlay a few random translucent rectangles for extra variability
	rectCount := 2 + r.Intn(3) // 2-4 overlays
	for i := 0; i < rectCount; i++ {
		rw := w/6 + r.Intn(w/4)
		rh := h/8 + r.Intn(h/5)
		rx := r.Intn(max(1, w-rw))
		ry := r.Intn(max(1, h-rh))
		c := color.RGBA{R: uint8(r.Intn(256)), G: uint8(r.Intn(256)), B: uint8(r.Intn(256)), A: uint8(16 + r.Intn(40))}
		drawRect(rgba, rx, ry, rw, rh, c)
	}
	return rgba
}

func drawRect(dst *image.RGBA, x, y, w, h int, c color.RGBA) {
	rect := image.Rect(x, y, x+w, y+h)
	draw.Draw(dst, rect, &image.Uniform{C: c}, image.Point{}, draw.Over)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func cryptoSeed() int64 {
	var b [8]byte
	if _, err := crand.Read(b[:]); err != nil {
		return time.Now().UnixNano()
	}
	return int64(binary.LittleEndian.Uint64(b[:]))
}
