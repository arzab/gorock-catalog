// Package rockhttp is an HTTP transport for rockhandlers on net/http.
//
// EXPERIMENTAL: its API may change.
//
//	server := rockhttp.New(cfg,
//	    rockhttp.Endpoint{Method: "POST", Path: "/tasks", Handler: tasks.Create, Status: http.StatusCreated},
//	    rockhttp.Endpoint{Method: "GET", Path: "/tasks/{id}", Handler: tasks.Get},
//	).Use(Log)
//	engine.MustRegister("http", server)
package rockhttp

import "github.com/arzab/gorock-kit/rockhandlers"

// Endpoint binds a handler chain to an HTTP method and path.
type Endpoint struct {
	Method  string                 // GET, POST, ...
	Path    string                 // net/http pattern: "/tasks/{id}"
	Handler *rockhandlers.Pipeline // the chain: rockhandlers.Chain(...)
	Status  int                    // success status; default 200, or 204 for an empty response struct
}
