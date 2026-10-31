// Package intel talks to the Python advisory service.
//
// The client is intentionally thin: one POST, a short timeout, and a hard rule
// that a missing service is not an error. Advisory data is a bonus, never a
// precondition for a useful scan.
package intel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"phantomdep/internal/model"
)

// DefaultAddr is the loopback address of the Python stage.
const DefaultAddr = "127.0.0.1:8788"

// Entry is one package's intel record.
type Entry struct {
	Ecosystem  string           `json:"ecosystem"`
	Name       string           `json:"name"`
	License    string           `json:"license"`
	Deprecated bool             `json:"deprecated"`
	Advisories []model.Advisory `json:"advisories"`
}

type request struct {
	Items []item `json:"items"`
}

type item struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
}

type response struct {
	Items []Entry `json:"items"`
	Error string  `json:"error"`
}

var client = &http.Client{Timeout: 1200 * time.Millisecond}

// Probe reports whether the service answers on addr within the timeout.
func Probe(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// Fetch resolves licences and advisories for the given packages (R5.1).
func Fetch(addr string, pkgs []model.Package) (map[string]Entry, error) {
	out := map[string]Entry{}
	if len(pkgs) == 0 {
		return out, nil
	}

	req := request{}
	for _, p := range pkgs {
		if p.Ecosystem == "root" {
			continue
		}
		req.Items = append(req.Items, item{Ecosystem: p.Ecosystem, Name: p.Name})
	}
	body, err := json.Marshal(req)
	if err != nil {
		return out, err
	}

	url := fmt.Sprintf("http://%s/intel", addr)
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, errors.New("intel service returned " + resp.Status)
	}

	var decoded response
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return out, err
	}
	if decoded.Error != "" {
		return out, errors.New(decoded.Error)
	}
	for _, e := range decoded.Items {
		out[e.Ecosystem+":"+e.Name] = e
	}
	return out, nil
}
