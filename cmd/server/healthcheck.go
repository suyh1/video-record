package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

func healthcheck(ctx context.Context, portValue string, client *http.Client) error {
	if portValue == "" {
		portValue = "8080"
	}
	port, err := strconv.Atoi(portValue)
	if err != nil || port < 1 || port > 65535 || client == nil {
		return errors.New("invalid healthcheck configuration")
	}
	request, err := http.NewRequestWithContext(
		ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/readyz", port), nil,
	)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("service is not ready: status %d", response.StatusCode)
	}
	return nil
}
