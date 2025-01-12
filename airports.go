package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/golang/geo/s2"
	alphafoxtrot "github.com/grumpypixel/go-airport-finder"
	"github.com/labstack/echo/v4"
	slogecho "github.com/samber/slog-echo"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"golang.org/x/sync/errgroup"

	"go.askask.com/locationcode/types"
)

type Finder struct {
	f *alphafoxtrot.AirportFinder
}

func main() {
	log := logger.Setup()

	dataDir := flag.String("data-dir", "./data", "Data cache directory")

	flag.Parse()

	err := validateData(*dataDir)
	if err != nil {
		log.Error("data error", "err", err)
		os.Exit(2)
	}

	finder := &Finder{
		f: alphafoxtrot.NewAirportFinder(),
	}

	// LoadOptions come with preset filepaths
	options := alphafoxtrot.PresetLoadOptions(*dataDir)

	// filter := alphafoxtrot.AirportTypeLarge | alphafoxtrot.AirportTypeMedium
	filter := alphafoxtrot.AirportTypeRunways

	// Load the data into memory
	if err := finder.f.Load(options, filter); len(err) > 0 {
		log.Error("finder load error", "err", err)
	}

	if args := flag.Args(); len(args) > 0 {
		if len(args) != 4 {
			fmt.Printf("[cc] [lat] [lng] [radius]\n")
			os.Exit(2)
		}
		cc := strings.ToUpper(args[0])

		latitude, err := strconv.ParseFloat(args[1], 64)
		if err != nil {
			fmt.Printf("could not parse %s as latitude: %s", args[1], err)
			os.Exit(2)
		}

		longitude, err := strconv.ParseFloat(args[2], 64)
		if err != nil {
			fmt.Printf("could not parse %s as longitude: %s", args[2], err)
			os.Exit(2)
		}

		radiusKM, err := strconv.ParseFloat(args[3], 64)
		if err != nil {
			fmt.Printf("could not parse %s as radius: %s", args[3], err)
			os.Exit(2)
		}

		airports, err := finder.GetAirports(context.Background(), cc, radiusKM, latitude, longitude)
		if err != nil {
			fmt.Printf("GetAirports error: %s\n", err)
		}

		for _, a := range airports {
			fmt.Printf("%s\t%s (%0f)\n", a.Code, a.Name, a.Distance)
		}

		os.Exit(0)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	tpShutdown, err := tracing.InitTracer(ctx, &tracing.TracerConfig{
		ServiceName: "locationcode",
	})
	if err != nil {
		log.Error("could not initialize tracer", "err", err)
	}

	e := echo.New()
	e.Use(otelecho.Middleware("locationcode"))

	e.Use(slogecho.NewWithConfig(log,
		slogecho.Config{
			WithTraceID:      false, // done by logger already
			DefaultLevel:     slog.LevelInfo,
			ClientErrorLevel: slog.LevelWarn,
			ServerErrorLevel: slog.LevelError,
			// WithRequestHeader: true,

			Filters: []slogecho.Filter{
				func(c echo.Context) bool {
					return !(c.Request().URL.Path == "/" || c.Request().URL.Path == "/__healthz")
				},
			},
		},
	))

	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "location code service")
	})
	e.GET("/v1/code", func(c echo.Context) error {
		ctx := c.Request().Context()
		countryISOCode := c.QueryParam("cc")
		countryISOCode = strings.ToUpper(countryISOCode)

		radiusString := c.QueryParam("radius")

		var radiusKM float64
		if len(radiusString) > 0 {
			radiusKM, _ = strconv.ParseFloat(radiusString, 64)
		}

		if radiusKM < 150 {
			radiusKM = 150
		} else {
			radiusKM = radiusKM * 1.5
		}

		latitude, longitude, err := getLatLng(c)
		if err != nil {
			return c.String(http.StatusBadRequest, fmt.Sprintf("invalid lat or lng: %s", err))
		}

		airports, err := finder.GetAirports(ctx, countryISOCode, radiusKM, latitude, longitude)
		if err != nil {
			return err
		}

		return c.JSON(http.StatusOK, airports)
	})

	errg, ctx := errgroup.WithContext(ctx)

	errg.Go(func() error {
		return e.Start(":8000")
	})

	errg.Go(func() error {
		<-ctx.Done()
		log.InfoContext(ctx, "shutting down server")
		return e.Shutdown(ctx)
	})

	err = errg.Wait()
	if err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			log.ErrorContext(ctx, "server error", "err", err)
		}
	}

	shutdownCtx, shCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shCancel()
	if err := tpShutdown(shutdownCtx); err != nil {
		log.ErrorContext(shutdownCtx, "failed to shutdown tracer", "err", err)
	}
}

func (f *Finder) GetAirports(ctx context.Context, cc string, radiusKM, latitude, longitude float64) ([]*types.Airport, error) {
	log := logger.FromContext(ctx)
	log.InfoContext(ctx, fmt.Sprintf("GetAirports(%s %.2f %.4f %.4f)", cc, radiusKM, latitude, longitude))

	ipLocation := s2.LatLngFromDegrees(latitude, longitude)

	maxResults := 500 // some countries have a lot of airports without local codes
	radiusInMeters := radiusKM * 1000
	airportsRaw := f.f.FindNearestAirportsByCountry(
		cc,
		latitude, longitude,
		radiusInMeters,
		maxResults,
		alphafoxtrot.AirportTypeAll, // filtered at load time
	)

	airports := []*alphafoxtrot.Airport{}
	for _, ap := range airportsRaw {
		if len(ap.IATACode) == 0 {
			continue
		}
		airports = append(airports, ap)
	}

	r := []*types.Airport{}

	for _, airport := range airports {
		// fmt.Printf("%d %s: %+v\n", i, airport.Name, airport)

		a := types.NewAirport(airport)

		ll := s2.LatLngFromDegrees(airport.LatitudeDeg, airport.LongitudeDeg)
		a.Distance = float64(ipLocation.Distance(ll)) * 6371.01

		r = append(r, a)
	}

	types.SortAirports(r)

	maxReturns := 20
	if len(r) > maxReturns {
		r = r[0:maxReturns]
	}

	log.InfoContext(ctx, "got airports", "count", len(airportsRaw), "filtered", len(airports), "returning", len(r))

	return r, nil
}

func validateData(dataDir string) error {
	downloadFiles := false
	for _, filename := range alphafoxtrot.OurAirportsFiles {
		filepath := path.Join(dataDir, filename)
		if _, err := os.Stat(filepath); os.IsNotExist(err) {
			downloadFiles = true
			break
		}
	}
	if downloadFiles {
		fmt.Println("Downloading CSV files from OurAirports.com...")
		err := DownloadDatabase(dataDir)
		if err != nil {
			return err
		}
	}
	return nil
}

func getLatLng(c echo.Context) (float64, float64, error) {
	latitudeStr := c.QueryParam("lat")  //  37.3793
	longitudeStr := c.QueryParam("lng") // -122.12

	latitude, err := strconv.ParseFloat(latitudeStr, 64)
	if err != nil {
		return 0, 0, err
	}
	longitude, err := strconv.ParseFloat(longitudeStr, 64)
	if err != nil {
		return 0, 0, err
	}

	return latitude, longitude, nil
}
