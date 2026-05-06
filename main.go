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

// ImagePair represents a single image copy operation.
type ImagePair struct {
	Src  string `json:"src"`
	Dest string `json:"dest"`
}

// CopyRequest is the expected JSON body for /invoke.
type CopyRequest struct {
	Images        []ImagePair `json:"images"`
	MaxConcurrent int         `json:"max_concurrent"`
	SrcUser       string      `json:"src_user"`
	SrcPwd        string      `json:"src_pwd"`
	DestUser      string      `json:"dest_user"`
	DestPwd       string      `json:"dest_pwd"`
}

// CopyResult reports the outcome of a single image copy.
type CopyResult struct {
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

func parseRegistry(ref string) (string, error) {
	r, err := name.ParseReference(ref)
	if err != nil {
		return "", err
	}
	return r.Context().RegistryStr(), nil
}

func main() {
	r := gin.Default()

	r.POST("/invoke", func(c *gin.Context) {
		var req CopyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if len(req.Images) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "images list is empty"})
			return
		}

		if req.MaxConcurrent <= 0 {
			req.MaxConcurrent = 3
		}

		// Determine canonical registry names from the first image pair.
		var srcRegistry, destRegistry string
		if len(req.Images) > 0 {
			var err error
			srcRegistry, err = parseRegistry(req.Images[0].Src)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid src image: %v", err)})
				return
			}
			destRegistry, err = parseRegistry(req.Images[0].Dest)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid dest image: %v", err)})
				return
			}
		}

		// Build authenticators.
		srcAuth := authn.Anonymous
		if req.SrcUser != "" {
			srcAuth = &authn.Basic{Username: req.SrcUser, Password: req.SrcPwd}
		}
		destAuth := authn.Anonymous
		if req.DestUser != "" {
			destAuth = &authn.Basic{Username: req.DestUser, Password: req.DestPwd}
		}

		kc := &registryKeychain{
			srcRegistry:  srcRegistry,
			srcAuth:      srcAuth,
			destRegistry: destRegistry,
			destAuth:     destAuth,
		}

		results := make([]CopyResult, len(req.Images))
		var mu sync.Mutex

		g, ctx := errgroup.WithContext(context.Background())
		g.SetLimit(req.MaxConcurrent)

		for i, img := range req.Images {
			i, img := i, img // capture loop vars
			g.Go(func() error {
				opts := []crane.Option{
					crane.WithAuthFromKeychain(kc),
					crane.WithContext(ctx),
				}
				if err := crane.Copy(img.Src, img.Dest, opts...); err != nil {
					mu.Lock()
					results[i] = CopyResult{Src: img.Src, Dest: img.Dest, Error: err.Error()}
					mu.Unlock()
					return nil // continue processing other images
				}
				mu.Lock()
				results[i] = CopyResult{Src: img.Src, Dest: img.Dest}
				mu.Unlock()
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"results": results})
	})

	r.POST("/initialize", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "initialize ok"})
	})

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "hello world"})
	})

	_ = r.Run(":8080")
}
