package locationcode

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"go.askask.com/locationcode/types"
)

var client http.Client

func init() {
	netTransport := &http.Transport{
		Dial: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 5 * time.Second,
	}
	client = http.Client{
		Timeout:   time.Second * 10,
		Transport: netTransport,
	}
}

func GetAirports(ctx context.Context, countryCode string, lat, lng float64, radiusKM float64) ([]*types.Airport, error) {
	ctx, span := otel.Tracer("locationcode").Start(ctx, "locationcode.GetAirports")
	defer span.End()

	baseURL := os.Getenv("locationcode_service")
	if len(baseURL) == 0 {
		return nil, fmt.Errorf("locationcode_service not configured")
	}

	q := url.Values{}
	q.Set("cc", countryCode)
	if radiusKM > 0 {
		q.Set("radius", fmt.Sprintf("%f", radiusKM))
	}
	q.Set("lat", fmt.Sprintf("%f", lat))
	q.Set("lng", fmt.Sprintf("%f", lng))

	reqURL, err := url.Parse(fmt.Sprintf("http://%s/v1/code?%s", baseURL, q.Encode()))
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(attribute.String("url", reqURL.String()))

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL.String(), nil)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	defer resp.Body.Close()

	// io.Copy(os.Stdout, resp.Body)
	// return nil, nil

	dec := json.NewDecoder(resp.Body)

	airports := []*types.Airport{}
	err = dec.Decode(&airports)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	return airports, nil
}
