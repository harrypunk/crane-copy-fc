package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"golang.org/x/sync/errgroup"
)

// CopyRequest is the expected JSON body for /invoke.
type CopyRequest struct {
	SrcPrefix     string      `json:"src_prefix"`
	DestPrefix    string      `json:"dest_prefix"`
	Images        [][2]string `json:"images"`
	MaxConcurrent int         `json:"max_concurrent"`
	SrcUser       string      `json:"src_user"`
	SrcPwd        string      `json:"src_pwd"`
	DestUser      string      `json:"dest_user"`
	DestPwd       string      `json:"dest_pwd"`
}

// CopyResult reports the outcome of a single image copy.
type CopyResult struct {
	Image string `json:"image"`
	Src   string `json:"src"`
	Dest  string `json:"dest"`
	Error string `json:"error,omitempty"`
}

// registryKeychain returns different credentials based on the target registry.
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

func joinImage(prefix, image string) string {
	if prefix == "" {
		return image
	}
	if prefix[len(prefix)-1] == '/' {
		return prefix + image
	}
	return prefix + "/" + image
}

func parseRegistry(ref string) (string, error) {
	r, err := name.ParseReference(ref)
	if err != nil {
		return "", err
	}
	return r.Context().RegistryStr(), nil
}

func buildAuth(user, pwd string) authn.Authenticator {
	if user == "" {
		return authn.Anonymous
	}
	return &authn.Basic{Username: user, Password: pwd}
}

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

func buildKeychain(srcRegistry, destRegistry, srcUser, srcPwd, destUser, destPwd string) *registryKeychain {
	return &registryKeychain{
		srcRegistry:  srcRegistry,
		srcAuth:      buildAuth(srcUser, srcPwd),
		destRegistry: destRegistry,
		destAuth:     buildAuth(destUser, destPwd),
	}
}

func copyImage(ctx context.Context, kc *registryKeychain, srcRef, destRef string) error {
	opts := []crane.Option{
		crane.WithAuthFromKeychain(kc),
		crane.WithContext(ctx),
	}
	return crane.Copy(srcRef, destRef, opts...)
}

func doCopy(req CopyRequest) ([]CopyResult, error) {
	srcRegistry, destRegistry, err := resolveRegistries(&req)
	if err != nil {
		return nil, err
	}

	if req.MaxConcurrent <= 0 {
		req.MaxConcurrent = 3
	}

	kc := buildKeychain(srcRegistry, destRegistry, req.SrcUser, req.SrcPwd, req.DestUser, req.DestPwd)
	results := make([]CopyResult, len(req.Images))
	var mu sync.Mutex

	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(req.MaxConcurrent)

	for i, pair := range req.Images {
		i, pair := i, pair // capture loop vars
		g.Go(func() error {
			srcRef := joinImage(req.SrcPrefix, pair[0])
			destRef := joinImage(req.DestPrefix, pair[1])
			label := pair[0] + " -> " + pair[1]

			if err := copyImage(ctx, kc, srcRef, destRef); err != nil {
				mu.Lock()
				results[i] = CopyResult{Image: label, Src: srcRef, Dest: destRef, Error: err.Error()}
				mu.Unlock()
				return nil // continue processing other images
			}
			mu.Lock()
			results[i] = CopyResult{Image: label, Src: srcRef, Dest: destRef}
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return results, nil
}

func handleInvoke(c *gin.Context) {
	var req CopyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	results, err := doCopy(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"results": results})
}

func main() {
	r := gin.Default()

	r.POST("/invoke", handleInvoke)

	r.POST("/initialize", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "initialize ok"})
	})

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "hello world"})
	})

	if err := r.Run(":8080"); err != nil {
		panic(err)
	}
}
