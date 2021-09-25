package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mcdexio/mai3-trade-mining-watcher/common/logging"
)

type Client struct {
	client *http.Client
	logger logging.Logger
}

func assertHttpInterface() {
	var _ IHttpClient = (*Client)(nil)
}

type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func NewHttpClient(transport *http.Transport, logger logging.Logger) *Client {
	if transport == nil {
		transport = http.DefaultTransport.(*http.Transport)
	}

	return &Client{
		client: &http.Client{Transport: transport},
		logger: logger,
	}
}

const ErrorCode = -1

func (h *Client) Request(method, u string, params []KeyValue, requestBody interface{}, headers []KeyValue) (err error, code int, respBody []byte) {
	start := time.Now().UTC()
	code = ErrorCode
	respBody = []byte{}
	err = nil
	defer func() {
		// h.logger.Debug("###[%s]### cost[%.0f] %s %v %v %v ###[%d]###response###%s", method, float64(time.Since(start))/1000000, u, requestBody, params, headers, code, string(respBody))
		h.logger.Debug("###[%s]### cost[%.0f] %s %v %v %v ###[%d]###response###", method, float64(time.Since(start))/1000000, u, requestBody, params, headers, code)
	}()

	if len(u) == 0 {
		err = fmt.Errorf("url is empty")
		h.logger.Error(err.Error())
		return
	}

	_, err = url.Parse(u)
	if err != nil {
		h.logger.Error("parse url %s failed, error: %v", u, err)
		return
	}

	var buffer bytes.Buffer
	buffer.WriteString(u)
	if len(params) > 0 && !strings.HasSuffix(u, "?") {
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

func (h *Client) Get(url string, params []KeyValue, body interface{}, header []KeyValue) (error, int, []byte) {
	return h.Request(http.MethodGet, url, params, body, header)
}

func (h *Client) Post(url string, params []KeyValue, body interface{}, header []KeyValue) (error, int, []byte) {
	return h.Request(http.MethodPost, url, params, body, header)
}

func (h *Client) Delete(url string, params []KeyValue, body interface{}, header []KeyValue) (error, int, []byte) {
	return h.Request(http.MethodDelete, url, params, body, header)
}

func (h *Client) Put(url string, params []KeyValue, body interface{}, header []KeyValue) (error, int, []byte) {
	return h.Request(http.MethodPut, url, params, body, header)
}

func closeBody(resp *http.Response, logger logging.Logger) {
	if resp != nil && resp.Body != nil {
		err := resp.Body.Close()
		if err != nil {
			logger.Error("response body close error: %v, req: %v", err.Error(), resp.Request)
		}
	}
}
