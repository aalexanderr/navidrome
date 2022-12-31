package public

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth/v5"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/navidrome/navidrome/core/artwork"
	"github.com/navidrome/navidrome/core/auth"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server"
)

type Router struct {
	http.Handler
	artwork artwork.Artwork
}

func New(artwork artwork.Artwork) *Router {
	p := &Router{artwork: artwork}
	p.Handler = p.routes()

	t, err := auth.CreatePublicToken(map[string]any{
		"id":   "al-ee07551e7371500da55e23ae8520f1d8",
		"size": 300,
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("!!!!!!!!!!!!!!!!!", t, "!!!!!!!!!!!!!!!!")

	return p
}

func (p *Router) routes() http.Handler {
	r := chi.NewRouter()

	r.Group(func(r chi.Router) {
		r.Use(server.URLParamsMiddleware)
		r.Use(jwtVerifier)
		r.Use(validator)
		r.Get("/img/{jwt}", p.handleImages)
	})
	return r
}

func (p *Router) handleImages(w http.ResponseWriter, r *http.Request) {
	_, claims, _ := jwtauth.FromContext(r.Context())
	id, ok := claims["id"].(string)
	if !ok {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	size, ok := claims["size"].(float64)
	if !ok {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	imgReader, lastUpdate, err := p.artwork.Get(r.Context(), id, int(size))
	w.Header().Set("cache-control", "public, max-age=315360000")
	w.Header().Set("last-modified", lastUpdate.Format(time.RFC1123))

	switch {
	case errors.Is(err, context.Canceled):
		return
	case errors.Is(err, model.ErrNotFound):
		log.Error(r, "Couldn't find coverArt", "id", id, err)
		http.Error(w, "Artwork not found", http.StatusNotFound)
		return
	case err != nil:
		log.Error(r, "Error retrieving coverArt", "id", id, err)
		http.Error(w, "Error retrieving coverArt", http.StatusInternalServerError)
		return
	}

	defer imgReader.Close()
	_, err = io.Copy(w, imgReader)
}

func jwtVerifier(next http.Handler) http.Handler {
	return jwtauth.Verify(auth.TokenAuth, func(r *http.Request) string {
		return r.URL.Query().Get(":jwt")
	})(next)
}

func validator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, _, err := jwtauth.FromContext(r.Context())

		validErr := jwt.Validate(token,
			jwt.WithRequiredClaim("id"),
			jwt.WithRequiredClaim("size"),
		)
		if err != nil || token == nil || validErr != nil {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}

		// Token is authenticated, pass it through
		next.ServeHTTP(w, r)
	})
}
