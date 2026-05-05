package httpclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

var httpClient = &http.Client{}

func PostJSON(url string, headers map[string]string, body, out interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		msg := string(raw)
		if readErr != nil {
			msg = fmt.Sprintf("(body read error: %v)", readErr)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
	}
	if readErr != nil {
		return fmt.Errorf("reading response body: %w", readErr)
	}
	return json.Unmarshal(raw, out)
}
