package content

import (
	"fmt"
	"net/http"
	"time"
)

type ScrappingClientConfig struct {
	Timeout       time.Duration
	WithProxy     bool
	UseProxyFirst bool
}

func (s *Service) newClient(cfg ScrappingClientConfig) *ScrappingYoungLad {
	ua := UserAgent
	var proxyURL *string
	if cfg.WithProxy && s.ProxyURL != "" {
		p := s.ProxyURL
		proxyURL = &p
	}
	return &ScrappingYoungLad{
		Client:        &http.Client{Timeout: cfg.Timeout},
		ProxyURL:      proxyURL,
		UseProxyFirst: cfg.UseProxyFirst,
		UserAgent:     &ua,
	}
}

type ScrappingYoungLad struct {
	Client        *http.Client
	ProxyURL      *string
	UseProxyFirst bool
	UserAgent     *string
}

func (c *ScrappingYoungLad) doDirectRequest(req *http.Request) (*http.Response, error) {
	fmt.Printf("attempting direct request to %s\n", req.URL.String())
	return c.Client.Do(req)
}

func (c *ScrappingYoungLad) doProxyRequest(req *http.Request) (*http.Response, error) {
	proxyReq, err := http.NewRequest(req.Method, *c.ProxyURL+"?url="+req.URL.String(), req.Body)
	if err != nil {
		fmt.Printf("failed to create proxy request for %s: %v\n", req.URL.String(), err)
		return nil, err
	}
	proxyReq.Header = req.Header.Clone()
	fmt.Printf("attempting proxy request to %s via %s\n", req.URL.String(), *c.ProxyURL)
	return c.Client.Do(proxyReq)
}

func (c *ScrappingYoungLad) generateDirectAndProxyError(directErr, proxyErr error) error {
	return fmt.Errorf("both direct and proxy requests failed: direct error: %v, proxy error: %v", directErr, proxyErr)
}

func (c *ScrappingYoungLad) isSuccessfulResponse(resp *http.Response, err error) bool {
	if err != nil || resp == nil {
		return false
	}
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func (c *ScrappingYoungLad) Do(req *http.Request) (*http.Response, error) {
	if c.UserAgent != nil {
		req.Header.Set("User-Agent", *c.UserAgent)
	}
	if c.ProxyURL == nil {
		return c.doDirectRequest(req)
	}

	if c.UseProxyFirst {
		resp, proxyErr := c.doProxyRequest(req)
		if c.isSuccessfulResponse(resp, proxyErr) {
			return resp, nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		resp, directErr := c.doDirectRequest(req)
		if c.isSuccessfulResponse(resp, directErr) {
			return resp, nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		return nil, c.generateDirectAndProxyError(directErr, proxyErr)
	}
	resp, directErr := c.doDirectRequest(req)
	if c.isSuccessfulResponse(resp, directErr) {
		return resp, nil
	}
	if resp != nil {
		resp.Body.Close()
	}
	resp, proxyErr := c.doProxyRequest(req)
	if c.isSuccessfulResponse(resp, proxyErr) {
		return resp, nil
	}
	if resp != nil {
		resp.Body.Close()
	}
	return nil, c.generateDirectAndProxyError(directErr, proxyErr)
}
