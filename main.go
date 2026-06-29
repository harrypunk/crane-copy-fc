package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/logs"
	"github.com/google/go-containerregistry/pkg/name"
	"golang.org/x/sync/errgroup"
)

func init() {
	// Enable crane progress output so layer pull/push progress is visible.
	logs.Progress = log.New(os.Stdout, "crane: ", log.LstdFlags)
}

// CopyRequest is the expected JSON body for /invoke.
type CopyRequest struct {
	SrcPrefix     string      `json:"src_prefix"`
	DestPrefix    string      `json:"dest_prefix"`
	Images        [][2]string `json:"images"` // Each element is [src_suffix, dest_suffix]. Empty dest_suffix defaults to src_suffix.
	MaxConcurrent int         `json:"max_concurrent"` // Max concurrent HTTP jobs while copying a single image (layer pulls/pushes).
	SrcUser       string      `json:"src_user"`
	SrcPwd        string      `json:"src_pwd"`
	DestUser      string      `json:"dest_user"`
	DestPwd       string      `json:"dest_pwd"`
}

// CopyResult reports the outcome of a single image copy.
type CopyResult struct {
	Image string `json:"image"` // "src -> dest" label for display
	Src   string `json:"src"`   // Fully resolved source reference
	Dest  string `json:"dest"`  // Fully resolved destination reference
	Error string `json:"error,omitempty"`
}

// registryKeychain implements authn.Keychain so that crane.Copy can use
// different credentials for the source registry and the destination registry.
type registryKeychain struct {
	srcRegistry  string
	srcAuth      authn.Authenticator
	destRegistry string
	destAuth     authn.Authenticator
}

func (rk *registryKeychain) Resolve(target authn.Resource) (authn.Authenticator, error) {
	if target.RegistryStr() == rk.srcRegistry {
		return rk.srcAuth, nil
	}
	if target.RegistryStr() == rk.destRegistry {
		return rk.destAuth, nil
	}
	return authn.Anonymous, nil
}

// joinImage concatenates prefix and image with a single '/' separator.
// If prefix is empty the image is returned as-is. Trailing slashes on prefix
// are handled so the result never contains "//".
func joinImage(prefix, image string) string {
	if prefix == "" {
		return image
	}
	if prefix[len(prefix)-1] == '/' {
		return prefix + image
	}
	return prefix + "/" + image
}

// parseRegistry extracts the canonical registry hostname from an image reference.
func parseRegistry(ref string) (string, error) {
	r, err := name.ParseReference(ref)
	if err != nil {
		return "", err
	}
	return r.Context().RegistryStr(), nil
}

// buildAuth returns an authn.Basic authenticator when credentials are provided,
// otherwise authn.Anonymous for public registries.
func buildAuth(user, pwd string) authn.Authenticator {
	if user == "" {
		return authn.Anonymous
	}
	return &authn.Basic{Username: user, Password: pwd}
}

// resolveRegistries parses the first image pair to determine the canonical
// source and destination registry names. These are used by the keychain to
// decide which credentials to send for pull vs push operations.
func resolveRegistries(req *CopyRequest) (srcRegistry, destRegistry string, err error) {
	if len(req.Images) == 0 {
		return "", "", fmt.Errorf("images list is empty")
	}
	if len(req.Images[0]) != 2 {
		return "", "", fmt.Errorf("each image must be a [src, dest] pair")
	}

	srcRef := joinImage(req.SrcPrefix, req.Images[0][0])
	if srcRegistry, err = parseRegistry(srcRef); err != nil {
		return "", "", fmt.Errorf("invalid src image: %w", err)
	}

	destRef := joinImage(req.DestPrefix, req.Images[0][1])
	if destRegistry, err = parseRegistry(destRef); err != nil {
		return "", "", fmt.Errorf("invalid dest image: %w", err)
	}

	return srcRegistry, destRegistry, nil
}

// buildKeychain assembles a registryKeychain with independent source/dest auth.
func buildKeychain(srcRegistry, destRegistry, srcUser, srcPwd, destUser, destPwd string) *registryKeychain {
	return &registryKeychain{
		srcRegistry:  srcRegistry,
		srcAuth:      buildAuth(srcUser, srcPwd),
		destRegistry: destRegistry,
		destAuth:     buildAuth(destUser, destPwd),
	}
}

// copyImage wraps crane.Copy with our keychain, context and per-image HTTP concurrency.
func copyImage(ctx context.Context, kc *registryKeychain, srcRef, destRef string, jobs int) error {
	opts := []crane.Option{
		crane.WithAuthFromKeychain(kc),
		crane.WithContext(ctx),
		crane.WithJobs(jobs),
	}
	return crane.Copy(srcRef, destRef, opts...)
}

// normalizeImages fills empty dest_suffix values with their corresponding
// src_suffix so callers can write ["nginx:1.25", ""] as a shorthand when
// the destination name should match the source.
func normalizeImages(images [][2]string) [][2]string {
	for i := range images {
		if images[i][1] == "" {
			images[i][1] = images[i][0]
		}
	}
	return images
}

// doCopy performs the actual image copies with controlled concurrency.
func doCopy(req CopyRequest) ([]CopyResult, error) {
	// Fill empty dest_suffix values before any further processing.
	req.Images = normalizeImages(req.Images)

	srcRegistry, destRegistry, err := resolveRegistries(&req)
	if err != nil {
		return nil, err
	}

	if req.MaxConcurrent <= 0 {
		req.MaxConcurrent = 3
	}

	log.Printf("[copy] src_registry=%s dest_registry=%s images=%d concurrent=%d",
		srcRegistry, destRegistry, len(req.Images), req.MaxConcurrent)

	kc := buildKeychain(srcRegistry, destRegistry, req.SrcUser, req.SrcPwd, req.DestUser, req.DestPwd)
	results := make([]CopyResult, len(req.Images))
	var mu sync.Mutex

	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(1) // copy images one at a time

	for i, pair := range req.Images {
		g.Go(func() error {
			srcRef := joinImage(req.SrcPrefix, pair[0])
			destRef := joinImage(req.DestPrefix, pair[1])
			label := pair[0] + " -> " + pair[1]

			log.Printf("[copy] starting %s", label)
			if err := copyImage(ctx, kc, srcRef, destRef, req.MaxConcurrent); err != nil {
				log.Printf("[copy] failed  %s: %v", label, err)
				mu.Lock()
				results[i] = CopyResult{Image: label, Src: srcRef, Dest: destRef, Error: err.Error()}
				mu.Unlock()
				return nil // continue processing other images
			}
			log.Printf("[copy] success %s", label)
			mu.Lock()
			results[i] = CopyResult{Image: label, Src: srcRef, Dest: destRef}
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	success, failed := 0, 0
	for _, r := range results {
		if r.Error == "" {
			success++
		} else {
			failed++
		}
	}
	log.Printf("[copy] done success=%d failed=%d", success, failed)

	return results, nil
}

// handleInvoke binds the JSON request and triggers the copy workflow.
func handleInvoke(c *gin.Context) {
	var req CopyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[invoke] bad request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	results, err := doCopy(req)
	if err != nil {
		log.Printf("[invoke] copy error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"results": results})
}

func main() {
	r := gin.Default()

	r.POST("/invoke", handleInvoke)

	r.POST("/initialize", func(c *gin.Context) {
		log.Println("[initialize] ok")
		c.JSON(http.StatusOK, gin.H{"message": "initialize ok"})
	})

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "hello world"})
	})

	log.Println("[server] starting on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("[server] failed: %v", err)
	}
}
