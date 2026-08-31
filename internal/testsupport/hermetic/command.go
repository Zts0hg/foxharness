package hermetic

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
)

/* CommandRequest identifies an exact fake command invocation. */
type CommandRequest struct {
	Dir  string
	Name string
	Args []string
	Env  map[string]string
}

/* CommandResponse is a controlled command outcome. */
type CommandResponse struct {
	Output   string
	ExitCode int
}

/* CommandExchange maps an exact command to a controlled outcome. */
type CommandExchange struct {
	Request  CommandRequest
	Response CommandResponse
	Error    string
}

/* CommandRunner is a finite fake command boundary. */
type CommandRunner struct {
	mu        sync.Mutex
	exchanges []CommandExchange
	next      int
	requests  []CommandRequest
}

/* NewCommandRunner copies command exchanges. */
func NewCommandRunner(exchanges []CommandExchange) *CommandRunner {
	return &CommandRunner{exchanges: append([]CommandExchange(nil), exchanges...)}
}

/* Run consumes one exact command exchange without starting a process. */
func (r *CommandRunner) Run(ctx context.Context, request CommandRequest) (CommandResponse, error) {
	select {
	case <-ctx.Done():
		return CommandResponse{}, ctx.Err()
	default:
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.next >= len(r.exchanges) {
		return CommandResponse{}, errors.New("command script exhausted")
	}
	exchange := r.exchanges[r.next]
	r.next++
	r.requests = append(r.requests, cloneCommandRequest(request))
	if !reflect.DeepEqual(exchange.Request, request) {
		return CommandResponse{}, fmt.Errorf("command request mismatch: got %#v want %#v", request, exchange.Request)
	}
	if exchange.Error != "" {
		return exchange.Response, errors.New(exchange.Error)
	}
	return exchange.Response, nil
}

/* Requests returns independent command request snapshots. */
func (r *CommandRunner) Requests() []CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	requests := make([]CommandRequest, len(r.requests))
	for i, request := range r.requests {
		requests[i] = cloneCommandRequest(request)
	}
	return requests
}

func cloneCommandRequest(request CommandRequest) CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Env = cloneStrings(request.Env)
	return request
}

func cloneStrings(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

/* GitHubRequest identifies a fake GitHub operation. */
type GitHubRequest struct {
	Operation  string
	Repository string
	Payload    string
}

/* GitHubResponse is one controlled GitHub result. */
type GitHubResponse struct {
	ID     string
	URL    string
	Status string
}

/* GitHubExchange maps one exact operation to a controlled result. */
type GitHubExchange struct {
	Request  GitHubRequest
	Response GitHubResponse
	Error    string
}

/* GitHub is a finite in-memory fake with no credentials or network client. */
type GitHub struct {
	mu        sync.Mutex
	exchanges []GitHubExchange
	next      int
	requests  []GitHubRequest
}

/* NewGitHub copies GitHub exchanges. */
func NewGitHub(exchanges []GitHubExchange) *GitHub {
	return &GitHub{exchanges: append([]GitHubExchange(nil), exchanges...)}
}

/* Do consumes one exact GitHub exchange. */
func (g *GitHub) Do(ctx context.Context, request GitHubRequest) (GitHubResponse, error) {
	select {
	case <-ctx.Done():
		return GitHubResponse{}, ctx.Err()
	default:
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.next >= len(g.exchanges) {
		return GitHubResponse{}, errors.New("GitHub script exhausted")
	}
	exchange := g.exchanges[g.next]
	g.next++
	g.requests = append(g.requests, request)
	if exchange.Request != request {
		return GitHubResponse{}, fmt.Errorf("GitHub request mismatch: got %#v want %#v", request, exchange.Request)
	}
	if exchange.Error != "" {
		return exchange.Response, errors.New(exchange.Error)
	}
	return exchange.Response, nil
}

/* Requests returns observed GitHub requests. */
func (g *GitHub) Requests() []GitHubRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]GitHubRequest(nil), g.requests...)
}
