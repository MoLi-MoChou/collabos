package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/collabos/collabos/v2/internal/wire"
)

// API is a small client for the relay's device API.
type API struct {
	Server string
	Token  string
	HTTP   *http.Client
}

func (a *API) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.Server+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.Token)
	req.Header.Set("Content-Type", "application/json")
	hc := a.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 3 * time.Minute}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(b, &e)
		if e.Error == "" {
			e.Error = string(b)
		}
		return fmt.Errorf("%s", e.Error)
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

// Agents lists devices.
func (a *API) Agents(ctx context.Context) ([]wire.Device, error) {
	var ds []wire.Device
	return ds, a.do(ctx, "GET", "/api/agents", nil, &ds)
}

// Create starts a call.
func (a *API) Create(ctx context.Context, r wire.CallRequest) (wire.Call, error) {
	var c wire.Call
	return c, a.do(ctx, "POST", "/api/calls", r, &c)
}

// Get long-polls a call for up to wait seconds or until it changes from since.
func (a *API) Get(ctx context.Context, id string, wait int, since string) (wire.Call, error) {
	var c wire.Call
	q := url.Values{"wait": {fmt.Sprint(wait)}}
	if since != "" {
		q.Set("since", since)
	}
	return c, a.do(ctx, "GET", "/api/calls/"+url.PathEscape(id)+"?"+q.Encode(), nil, &c)
}

// Cancel cancels a call.
func (a *API) Cancel(ctx context.Context, id string) (wire.Call, error) {
	var c wire.Call
	return c, a.do(ctx, "POST", "/api/calls/"+url.PathEscape(id)+"/cancel", nil, &c)
}

// List returns recent calls.
func (a *API) List(ctx context.Context, n int) ([]wire.Call, error) {
	var cs []wire.Call
	return cs, a.do(ctx, "GET", fmt.Sprintf("/api/calls?limit=%d", n), nil, &cs)
}

// Wait polls until the call ends or ctx/deadline expires, reporting progress.
func (a *API) Wait(ctx context.Context, id string, deadline time.Time, onProgress func(wire.Call)) (wire.Call, error) {
	since := ""
	var last wire.Call
	fails := 0
	for {
		left := int(time.Until(deadline).Seconds())
		if left <= 0 {
			return last, nil
		}
		w := min(left, 25)
		c, err := a.Get(ctx, id, w, since)
		if err != nil {
			if ctx.Err() != nil {
				return last, ctx.Err()
			}
			fails++
			if fails > 20 {
				return last, err
			}
			time.Sleep(2 * time.Second)
			continue
		}
		fails = 0
		last = c
		if wire.Terminal(c.Status) {
			return c, nil
		}
		if onProgress != nil {
			onProgress(c)
		}
		since = c.Status + "|" + c.Progress
	}
}
