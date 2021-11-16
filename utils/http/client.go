package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
)

type Client struct {
	client *http.Client
	logger logging.Logger
	url    string
}

func assertHttpInterface() {
	var _ IHttpClient = (*Client)(nil)
}

type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

var DefaultTransport = &http.Transport{
	DialContext: (&net.Dialer{
		Timeout: 500 * time.Millisecond,
	}).DialContext,
	TLSHandshakeTimeout: 1000 * time.Millisecond,
	MaxIdleConns:        100,
	IdleConnTimeout:     30 * time.Second,
}

func NewHttpClient(transport *http.Transport, logger logging.Logger, url string) *Client {
	if transport == nil {
		transport = http.DefaultTransport.(*http.Transport)
	}

	return &Client{
		client: &http.Client{Transport: transport},
		logger: logger,
		url:    url,
	}
}

const ErrorCode = -1

func (h *Client) Request(method string, params []KeyValue, requestBody interface{}, headers []KeyValue) (err error, code int, respBody []byte) {
	// start := time.Now().UTC()
	code = ErrorCode
	respBody = []byte{}
	err = nil
	defer func() {
		// h.logger.Debug("###[%s]### cost[%.0f] body %v param %v header %v ###[%d]###response###%s", method, float64(time.Since(start))/1000000, requestBody, params, headers, code, string(respBody))
	}()

	if len(h.url) == 0 {
		err = fmt.Errorf("url is empty")
		h.logger.Error(err.Error())
		return
	}

	_, err = url.Parse(h.url)
	if err != nil {
		h.logger.Error("parse url %s failed, error: %v", h.url, err)
		return
	}

	var buffer bytes.Buffer
	buffer.WriteString(h.url)
	if len(params) > 0 && !strings.HasSuffix(h.url, "?") {
		buffer.WriteString("?")
	}
	for i, param := range params {
		buffer.WriteString(url.QueryEscape(param.Key))
		buffer.WriteString("=")
		buffer.WriteString(url.QueryEscape(param.Value))
		if i < len(params)-1 {
			buffer.WriteString("&")
		}
	}

	var bodyBytes []byte
	if requestBody != nil {
		bodyBytes, err = json.Marshal(requestBody)
		if err != nil {
			h.logger.Error("build request error: %v", err.Error())
			return
		}
	}

	req, err := http.NewRequest(method, buffer.String(), bytes.NewBuffer(bodyBytes))
	if err != nil {
		h.logger.Error("build request error: %v", err.Error())
		return
	}

	req.Header.Set("Content-Type", "application/json")
	for _, header := range headers {
		req.Header.Set(header.Key, header.Value)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		h.logger.Error("http call error: %v", err.Error())
		return
	}

	bodyBytes, err = ioutil.ReadAll(resp.Body)
	defer closeBody(resp, h.logger)
	if err != nil {
		return
	} else {
		return nil, resp.StatusCode, bodyBytes
	}
}

func (h *Client) Get(params []KeyValue, body interface{}, header []KeyValue) (error, int, []byte) {
	return h.Request(http.MethodGet, params, body, header)
}

func (h *Client) Post(params []KeyValue, body interface{}, header []KeyValue) (error, int, []byte) {
	return h.Request(http.MethodPost, params, body, header)
}

func (h *Client) Delete(params []KeyValue, body interface{}, header []KeyValue) (error, int, []byte) {
	return h.Request(http.MethodDelete, params, body, header)
}

func (h *Client) Put(params []KeyValue, body interface{}, header []KeyValue) (error, int, []byte) {
	return h.Request(http.MethodPut, params, body, header)
}

func closeBody(resp *http.Response, logger logging.Logger) {
	if resp != nil && resp.Body != nil {
		err := resp.Body.Close()
		if err != nil {
			logger.Error("response body close error: %v, req: %v", err.Error(), resp.Request)
		}
	}
}
