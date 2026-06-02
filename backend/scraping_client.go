package main

import (
	"fmt"
	"net/http"
)

type ScrapingYoungLad struct {
	Client        *http.Client
	ProxyURL      *string
	UseProxyFirst bool
	UserAgent     *string
}

func (c *ScrapingYoungLad) doDirectRequest(req *http.Request) (*http.Response, error) {
	return c.Client.Do(req)
}

func (c *ScrapingYoungLad) doProxyRequest(req *http.Request) (*http.Response, error) {
	proxyReq, err := http.NewRequest(req.Method, *c.ProxyURL+"?url="+req.URL.String(), req.Body)
	if err != nil {
		return nil, err
	}
	proxyReq.Header = req.Header.Clone()
	return c.Client.Do(proxyReq)
}

func (c *ScrapingYoungLad) generateDirectAndProxyError(directErr, proxyErr error) error {
	return fmt.Errorf("both direct and proxy requests failed: direct error: %v, proxy error: %v", directErr, proxyErr)
}

func (c *ScrapingYoungLad) isSuccessfulResponse(resp *http.Response, err error) bool {
	if err != nil || resp == nil {
		return false
	}
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func (c *ScrapingYoungLad) Do(req *http.Request) (*http.Response, error) {
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
