package main

import (
  "flag"
  "fmt"
  "os"
  "strconv"
  "strings"
  "encoding/json"
  "encoding/csv"
  "io/ioutil"
  "path/filepath"
  "github.com/golang/geo/s2"
  "github.com/paulmach/go.geojson"
)


func main() {
  // Set up
  outputSeparateFiles := flag.Bool("separate", false, "Output Features into separate files")
  shouldIndent := flag.Bool("pretty", true, "Output pretty printend GeoJSON")
  skipCells := flag.Int("skipcells", 1000, "Skip features with more cells than this")
  maxLevel := flag.Int("maxlevel", 20, "MaxLevel setting for RegionCoverer")
  minLevel := flag.Int("minlevel", 5, "MinLevel setting for RegionCoverer")
  maxCells := flag.Int("maxcells", 1000, "MaxCells setting for RegionCoverer")
  outputDirectory := flag.String("outdir", "output", "Output directory")
  markerInputFilePath := flag.String("markers", "", "CSV of markers. Format: <name>,<latitude>,<longitude> Names containing a comma must be in quotes.")
  flag.Parse()
  fmt.Println("Separate:", *outputSeparateFiles)
  fmt.Println("Pretty:", *shouldIndent)
  fmt.Println("Skip cells:", *skipCells)
  fmt.Println("Max level:", *maxLevel)
  fmt.Println("Min level:", *minLevel)
  fmt.Println("Max cells:", *maxCells)
  fmt.Println("Markers:", *markerInputFilePath != "")
  fmt.Println("")
  inputFilePath := flag.Args()[0]
  inputFileName := filepath.Base(inputFilePath)
  // markerInputFileName := filepath.Base(*markerInputFilePath)
  os.MkdirAll(*outputDirectory, os.ModePerm)

  // Spaghetti for now
  var markerCellIds []*s2.CellID
  var markerFeatures []*geojson.Feature
  if *markerInputFilePath != "" {
    markerCellIds, markerFeatures = getMarkersFromCsv(*markerInputFilePath)
  } else {
    markerCellIds = []*s2.CellID{}
    markerFeatures = []*geojson.Feature{}
  }
  featureCollection := getFeatureCollectionFromGeojson(inputFilePath)
  for index, feature := range featureCollection.Features {
    relRole := "outer"
    if feature.Properties["@relations"] != nil {
      relRole = feature.Properties["@relations"].([]interface{})[0].(map[string]interface{})["role"].(string)
    }
    polygons := []*s2.Polygon{}
    holePolygons := []*s2.Polygon{}
    if feature.Geometry.IsPolygon() {
      outerPolygon, holePolygon := getS2PolygonFromGeojsonPolygon(feature.Geometry.Polygon)
      polygons = append(polygons, outerPolygon)
      holePolygons = append(holePolygons, holePolygon)
    }
    if feature.Geometry.IsLineString() {
      polygons = append(polygons, getS2PolygonFromGeojsonLineString(feature.Geometry.LineString))
    }
    if feature.Geometry.IsMultiPolygon() {
      for _, polygon := range feature.Geometry.MultiPolygon {
        outerPolygon, holePolygon := getS2PolygonFromGeojsonPolygon(polygon)
        polygons = append(polygons, outerPolygon)
        holePolygons = append(holePolygons, holePolygon)
      }
    }
    isHole := relRole == "inner"
    covering, cellIds, cellGeometry := getCoveringFromPolygons(polygons, isHole, *maxLevel, *minLevel, *maxCells)
    holeCovering, holeCellIds, holeCellGeometry := getCoveringFromPolygons(holePolygons, true, *maxLevel, *minLevel, *maxCells)

    if len(cellIds) > *skipCells || len(holeCellIds) > *skipCells {
      fmt.Println("Skipping", getPathForFeature(feature), len(cellIds), len(holeCellIds))
      continue
    }

    var cellFeature *geojson.Feature
    if len(cellIds) > 0 {
      cellFeature = geojson.NewMultiPolygonFeature(cellGeometry...)
      cellFeature.SetProperty("cellids", cellIds)
      cellFeature.SetProperty("stroke-width", 1)
      cellFeature.SetProperty("stroke-opacity", 1)
      cellFeature.SetProperty("fill-opacity", 0.3)
      if isHole {
        cellFeature.SetProperty("stroke", "#ff8080")
        cellFeature.SetProperty("fill", "#ff8080")
      } else {
        cellFeature.SetProperty("stroke", "#008000")
        cellFeature.SetProperty("fill", "#80ff80")
      }
    }

    var holeCellFeature *geojson.Feature
    if len(holeCellIds) > 0 {
      holeCellFeature = geojson.NewMultiPolygonFeature(holeCellGeometry...)
      holeCellFeature.SetProperty("holecellids", holeCellIds)
      holeCellFeature.SetProperty("stroke", "#ff8080")
      holeCellFeature.SetProperty("stroke-width", 1)
      holeCellFeature.SetProperty("stroke-opacity", 1)
      holeCellFeature.SetProperty("fill", "#ff8080")
      holeCellFeature.SetProperty("fill-opacity", 0.3)
    }

    checkContainedMarkerFeatures(covering, holeCovering, isHole, feature, markerCellIds, markerFeatures)

    if *outputSeparateFiles {
      var outputGeojsonData []byte
      var err error
      tempFeatureCollection := geojson.NewFeatureCollection()
      if len(cellIds) > 0 {
        tempFeatureCollection.AddFeature(cellFeature)
      }
      if len(holeCellIds) > 0 {
        tempFeatureCollection.AddFeature(holeCellFeature)
      }
      tempFeatureCollection.AddFeature(feature)
      if *shouldIndent {
        outputGeojsonData, err = json.MarshalIndent(tempFeatureCollection, "", " ")
      } else {
        outputGeojsonData, err = tempFeatureCollection.MarshalJSON()
      }
      check(err)
      err = ioutil.WriteFile(fmt.Sprintf("%s/%s_%05d.geojson", *outputDirectory, strings.Replace(getPathForFeature(feature), "/", "_", -1), index + 1), outputGeojsonData, 0644)
      check(err)
    } else {
      if len(cellIds) > 0 {
        featureCollection.AddFeature(cellFeature)
      }
      if len(holeCellIds) > 0 {
        featureCollection.AddFeature(holeCellFeature)
      }
    }
    if (index + 1) % 1000 == 0 {
      fmt.Println(fmt.Sprintf("Parsed %d Features", index + 1))
    }
  }
  fmt.Println(fmt.Sprintf("Parsed %d Features", len(featureCollection.Features)))

  if ! *outputSeparateFiles {
    var outputGeojsonData []byte
    var err error
    if *shouldIndent {
      outputGeojsonData, err = json.MarshalIndent(featureCollection, "", " ")
    } else {
      outputGeojsonData, err = featureCollection.MarshalJSON()
    }
    err = ioutil.WriteFile(fmt.Sprintf("%s/%s.geojson", *outputDirectory, strings.TrimSuffix(inputFileName, filepath.Ext(inputFileName))), outputGeojsonData, 0644)
    check(err)
  }

  outputContainedMarkersToCsv(markerFeatures, *outputDirectory)

  // End
  fmt.Println("Done")
}


func getFeatureCollectionFromGeojson(geojsonFilename string) *geojson.FeatureCollection {
  geojsonData, err := ioutil.ReadFile(geojsonFilename)
  check(err)
  featureCollection, err := geojson.UnmarshalFeatureCollection(geojsonData)
  check(err)
  return featureCollection
}


func getMarkersFromCsv(csvFilename string) ([]*s2.CellID, []*geojson.Feature) {
  cellIds := []*s2.CellID{}
  features := []*geojson.Feature{}
  for _, row := range readCsv(csvFilename) {
    name := row[0]
    lat, err := strconv.ParseFloat(row[1], 64)
    check(err)
    lng, err := strconv.ParseFloat(row[2], 64)
    check(err)
    latlng := s2.LatLngFromDegrees(lat, lng)
    cellId := s2.CellIDFromLatLng(latlng)
    cellIds = append(cellIds, &cellId)
    feature := geojson.NewPointFeature([]float64{lng, lat})
    feature.SetProperty("name", name)
    feature.SetProperty("cellid", cellId.ToToken())
    feature.SetProperty("within", []string{})
    features = append(features, feature)
  }
  return cellIds, features
}


func readCsv(csvFilename string) [][]string {
  csvFile, err := os.Open(csvFilename)
  check(err)
  defer csvFile.Close()
  reader := csv.NewReader(csvFile)
  reader.TrimLeadingSpace = true
  rows, err := reader.ReadAll()
  check(err)
  return rows
}


func outputContainedMarkersToCsv(features []*geojson.Feature, outputDirectory string) {
  csvFile, err := os.Create(fmt.Sprintf("%s/markers_within_features.csv", outputDirectory))
  check(err)
  defer csvFile.Close()
  writer := csv.NewWriter(csvFile)
  defer writer.Flush()
  for _, feature := range features {
    for _, within := range feature.Properties["within"].([]string) {
      err := writer.Write([]string{feature.Properties["name"].(string), within})
      check(err)
    }
  }
}


func checkContainedMarkerFeatures(
  coveringCellUnion *s2.CellUnion,
  holeCoveringCellUnion *s2.CellUnion,
  isMainFeatureHole bool,
  coveringFeature *geojson.Feature,
  markerCellIds []*s2.CellID,
  markerFeatures []*geojson.Feature) (
    []*s2.CellID,
    []*geojson.Feature) {
  containedMarkerCellIds := []*s2.CellID{}
  containedMarkerFeatures := []*geojson.Feature{}
  for index, markerCellId := range markerCellIds {
    if coveringCellUnion.ContainsCellID(*markerCellId) {
      withinText := getPathForFeature(coveringFeature)
      if holeCoveringCellUnion.ContainsCellID(*markerCellId) || isMainFeatureHole {
        withinText += " (hole)"
      }
      within := markerFeatures[index].Properties["within"]
      within = append(within.([]string), withinText)
      markerFeatures[index].SetProperty("within", within)
      containedMarkerCellIds = append(containedMarkerCellIds, markerCellIds[index])
      containedMarkerFeatures = append(containedMarkerFeatures, markerFeatures[index])
    }
  }
  return containedMarkerCellIds, containedMarkerFeatures
}


func getCoveringFromPolygons(polygons []*s2.Polygon, isHole bool, maxLevel int, minLevel int, maxCells int) (*s2.CellUnion, []string, [][][][]float64) {
  var covering s2.CellUnion
  var cellIds []string
  var cellGeometry [][][][]float64
  regionCoverer := &s2.RegionCoverer{MaxLevel: maxLevel, MinLevel: minLevel, MaxCells: maxCells}
  for _, polygon := range polygons {
    if isHole {
      covering = regionCoverer.InteriorCellUnion(polygon)
    } else {
      covering = regionCoverer.Covering(polygon)
    }
    for _, cellId := range covering {
      cellIds = append(cellIds, cellId.ToToken())
      cell := s2.CellFromCellID(cellId)
      vertices := [][]float64{}
      for k := 0; k < 5; k++ {
        vertex := cell.Vertex(k % 4)
        latlng := s2.LatLngFromPoint(vertex)
        vertices = append(vertices, []float64{float64(latlng.Lng.Degrees()), float64(latlng.Lat.Degrees())})
      }
      cellGeometry = append(cellGeometry, [][][]float64{vertices})
    }
  }
  return &covering, cellIds, cellGeometry
}


func getS2PolygonFromGeojsonPolygon(geojsonPolygon [][][]float64) (*s2.Polygon, *s2.Polygon) {
  var outerLoops []*s2.Loop
  var innerLoops []*s2.Loop
  for index, ring := range geojsonPolygon {
    // In GeoJSON, the first ring is an outer ring, the rest are holes
    isHole := index > 0
    loop := getS2LoopFromGeojsonRing(ring, isHole)
    if isHole {
      innerLoops = append(innerLoops, loop)
    } else {
      outerLoops = append(outerLoops, loop)
    }
  }
  return s2.PolygonFromLoops(outerLoops), s2.PolygonFromLoops(innerLoops)
}


func getS2PolygonFromGeojsonLineString(geojsonLineString [][]float64) *s2.Polygon {
  loop := getS2LoopFromGeojsonRing(geojsonLineString, false)
  return s2.PolygonFromLoops([]*s2.Loop{loop})
}


func getS2LoopFromGeojsonRing(ring [][]float64, isHole bool) *s2.Loop {
  var points []s2.Point
  for _, latlngMap := range ring {
    latlng := s2.LatLngFromDegrees(latlngMap[1], latlngMap[0])
    point := s2.PointFromLatLng(latlng)
    points = append(points, point)
  }
  // GeoJSON polygon rings must end with the starting point.
  // S2 polygons must not have identical vertices,
  // and do not need to end with the starting point.
  // We omit the last point, if it"s same as first.
  if points[0] == points[len(points) - 1] {
    points = points[:len(points) - 1]
  }
  // In GeoJSON holes must be clockwise, exteriors counterclockwise.
  // In S2 all loops must be counterclockwise.
  if isHole {
    points = reverseS2Points(points)
  }
  return s2.LoopFromPoints(points)
}


func getPathForFeature(feature *geojson.Feature) string {
  path := ""
  if feature.Properties["@relations"] != nil {
    path += fmt.Sprintf("relation/%d/", int(feature.Properties["@relations"].([]interface{})[0].(map[string]interface{})["rel"].(float64)))
  }
  path += feature.ID.(string)
  return path
}


func reverseS2Points(sp []s2.Point) []s2.Point {
  for i, j := 0, len(sp) - 1; i < j; i, j = i + 1, j - 1 {
    sp[i], sp[j] = sp[j], sp[i]
  }
  return sp
}


func check(e error) {
  if e != nil {
    panic(e)
  }
}
