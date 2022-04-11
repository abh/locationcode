package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/golang/geo/s2"
	alphafoxtrot "github.com/grumpypixel/go-airport-finder"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Airport struct {
	Name     string
	Code     string
	Distance float64
	data     *alphafoxtrot.Airport
}

func main() {

	var dataDir = flag.String("data-dir", "./data", "Data cache directory")

	flag.Parse()

	validateData(*dataDir)

	finder := alphafoxtrot.NewAirportFinder()

	// LoadOptions come with preset filepaths
	options := alphafoxtrot.PresetLoadOptions(*dataDir)

	// filter := alphafoxtrot.AirportTypeLarge | alphafoxtrot.AirportTypeMedium
	filter := alphafoxtrot.AirportTypeRunways

	// Load the data into memory
	if err := finder.Load(options, filter); len(err) > 0 {
		log.Println("errors:", err)
	}

	e := echo.New()

	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Skipper: func(c echo.Context) bool {
			return c.Request().URL.Path == "/" || c.Request().URL.Path == "/__healthz"
		},
	}))

	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "location code service")
	})
	e.GET("/v1/code", func(c echo.Context) error {

		countryISOCode := c.QueryParam("cc")
		countryISOCode = strings.ToUpper(countryISOCode)

		radiusString := c.QueryParam("radius")

		var radiusKM int
		if len(radiusString) > 0 {
			radiusKM, _ = strconv.Atoi(radiusString)
		}

		if radiusKM < 10 {
			radiusKM = 100
		}

		latitude, longitude, err := getLatLng(c)
		if err != nil {
			return c.String(http.StatusBadRequest, fmt.Sprintf("invalid lat or lng: %s", err))
		}

		ipLocation := s2.LatLngFromDegrees(latitude, longitude)

		maxResults := 100
		radiusInMeters := float64(radiusKM) * 1000
		airportsRaw := finder.FindNearestAirportsByCountry(countryISOCode, latitude, longitude, radiusInMeters, maxResults, filter)

		airports := []*alphafoxtrot.Airport{}
		for _, ap := range airportsRaw {
			if len(ap.IATACode) == 0 {
				continue
			}
			airports = append(airports, ap)
		}

		llCache := map[int]s2.LatLng{}
		for i, ap := range airports {
			ll := s2.LatLngFromDegrees(ap.LatitudeDeg, ap.LongitudeDeg)
			llCache[i] = ll
		}

		r := []*Airport{}

		for i, airport := range airports {
			// fmt.Printf("%d %s: %+v\n", i, airport.Name, airport)

			code := strings.ToLower(airport.Country.ISOCode + airport.IATACode)

			distance := float64(ipLocation.Distance(llCache[i])) * 6371.01

			a := &Airport{
				Name:     airport.Name,
				Code:     code,
				Distance: distance,
				data:     airport,
			}
			r = append(r, a)
		}

		sort.Slice(r, func(i, j int) bool {
			if r[i].data.Type == r[j].data.Type {
				return r[i].Distance < r[j].Distance
			}
			return airports[i].Type < airports[j].Type
		})

		if len(r) > 10 {
			r = r[0:10]
		}

		fmt.Printf("got %d airports, filtered to %d, returning %d\n", len(airportsRaw), len(airports), len(r))

		return c.JSON(http.StatusOK, r)
	})

	e.Logger.Fatal(e.Start(":8000"))

}

func validateData(dataDir string) {
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
		alphafoxtrot.DownloadDatabase(dataDir)
	}
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
