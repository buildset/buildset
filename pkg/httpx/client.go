package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"time"

	"github.com/buildset/buildset/pkg/reqid"
)

const (
	// maxErrorBytes bounds what is read from a failed response, so a dependency answering with
	// something enormous cannot be used to exhaust this process.
	maxErrorBytes = 8 << 10
	// maxDrainBytes is how much of a body is read back before closing, so the connection returns
	// to the pool instead of being dropped.
	maxDrainBytes = 4 << 10

	defaultTimeout             = 5 * time.Second
	defaultMaxIdleConnsPerHost = 32
)

type ClientOptions struct {
	// Timeout bounds one call. The default is five seconds, chosen against the thirty-second
	// write timeout of a page-serving process: the worst page makes two calls in sequence, so this
	// leaves room to render an error page rather than drop the browser's connection.
	Timeout time.Duration
	// MaxIdleConnsPerHost bounds the pool this client keeps to its one dependency.
	MaxIdleConnsPerHost int
}

// Client calls one service.
//
// A failed call is not retried. Most of these operations are writes, creating a post is not
// idempotent, and they sit on a browser request's critical path where a retry turns a slow
// dependency into a dead one by multiplying its load at the worst moment. The visitor's reload
// button is the retry. A caller that genuinely needs one, because the operation is idempotent and
// losing it is worse than repeating it, retries deliberately at its own call site.
type Client struct {
	service string
	baseURL string
	http    *http.Client
}

func NewClient(service, baseURL string, opts ClientOptions) (*Client, error) {
	if service == "" {
		return nil, fmt.Errorf("service name must not be empty")
	}

	if baseURL == "" {
		return nil, fmt.Errorf("base URL of %s must not be empty", service)
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	idle := opts.MaxIdleConnsPerHost
	if idle <= 0 {
		idle = defaultMaxIdleConnsPerHost
	}

	transport := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 2 * time.Second}).DialContext,
		MaxIdleConns:        idle,
		MaxIdleConnsPerHost: idle,
		IdleConnTimeout:     90 * time.Second,
		// Plain HTTP/1.1 on a private network. Multiplexing over one connection buys nothing here
		// and complicates the failure model.
		ForceAttemptHTTP2: false,
	}

	return &Client{
		service: service,
		baseURL: baseURL,
		http:    &http.Client{Timeout: timeout, Transport: transport},
	}, nil
}

// Call sends request as JSON to path and decodes the response into response, which may be nil for
// an operation that returns nothing.
//
// A domain failure comes back as *Error. Everything else comes back as an ordinary wrapped error,
// and a caller must not mistake one for the other.
func (c *Client) Call(ctx context.Context, path string, request, response any) error {
	body, err := json.Marshal(request)
	if err != nil {
		return c.wrap(path, fmt.Errorf("encode request: %w", err))
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return c.wrap(path, fmt.Errorf("build request: %w", err))
	}

	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")

	if id, ok := reqid.FromContext(ctx); ok {
		httpRequest.Header.Set(reqid.Header, id)
	}

	httpResponse, err := c.http.Do(httpRequest)
	if err != nil {
		return c.wrap(path, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(httpResponse.Body, maxDrainBytes))
		_ = httpResponse.Body.Close()
	}()

	if httpResponse.StatusCode != http.StatusOK {
		return c.wrap(path, decodeError(httpResponse))
	}

	if response == nil {
		return nil
	}

	if err := json.NewDecoder(httpResponse.Body).Decode(response); err != nil {
		return c.wrap(path, fmt.Errorf("decode response: %w", err))
	}

	return nil
}

// wrap names the service and the path and nothing else. The request body is never included: the
// identity service takes a live session token in one.
func (c *Client) wrap(path string, err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("%s %s: %w", c.service, path, err)
}

// decodeError is deliberately strict. Anything it does not positively recognise as a domain
// failure stays an ordinary error, which no errors.As against *Error will match.
//
// That is what keeps "the identity service is down" from being read as "no such session", which is
// how an access control gets bypassed by an outage.
func decodeError(response *http.Response) error {
	switch response.StatusCode {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict:
	default:
		return fmt.Errorf("unexpected status %d", response.StatusCode)
	}

	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return fmt.Errorf("status %d with content type %q", response.StatusCode, response.Header.Get("Content-Type"))
	}

	var envelope Envelope
	if err := json.NewDecoder(io.LimitReader(response.Body, maxErrorBytes)).Decode(&envelope); err != nil {
		return fmt.Errorf("status %d with undecodable body: %w", response.StatusCode, err)
	}

	if !envelope.Code.Known() {
		return fmt.Errorf("status %d with unknown code %q", response.StatusCode, envelope.Code)
	}

	return &Error{Code: envelope.Code, Message: envelope.Message}
}
