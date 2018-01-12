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


type Marker struct {
  cellId *s2.CellID
  feature *geojson.Feature
}


func main() {
  // Set up
  outputSeparateFiles := flag.Bool("separate", false, "Output Features into separate files")
  skipMarkerlessFeatures := flag.Bool("skipmarkerless", false, "Skip features with no markers within")
  skipFeaturelessMarkers := flag.Bool("skipfeatureless", false, "Skip markers not within features")
  excludeCellFeatures := flag.Bool("excludecellfeatures", false, "Exclude cell features (only useful when visualizing markers)")
  shouldIndent := flag.Bool("pretty", true, "Output pretty printend GeoJSON")
  maxCellFeatures := flag.Int("maxcellfeatures", 1000, "Skip features which generate more cells than this")
  maxLevel := flag.Int("maxlevel", 20, "MaxLevel setting for RegionCoverer")
  minLevel := flag.Int("minlevel", 5, "MinLevel setting for RegionCoverer")
  maxCells := flag.Int("maxcells", 1000, "MaxCells setting for RegionCoverer")
  gridLevel := flag.Int("grid", 0, "Add a grid of given level cells")
  outputDirectory := flag.String("outdir", "output", "Output directory")
  markerInputFilePath := flag.String("markers", "", "CSV of markers. Format: <name>,<latitude>,<longitude> Names containing a comma must be in quotes.")
  featureColor := flag.String("cf", "#7e7e7e", "Feature color")
  coverColor := flag.String("cc", "#008000", "Cover cells color")
  holeColor := flag.String("ch", "#ff8080", "Hole cells color")
  markerColor := flag.String("cm", "#7e7e7e", "Marker color")
  markerCoverColor := flag.String("cmc", "#008000", "Marker cover color")
  markerHoleColor := flag.String("cmh", "#ff8080", "Marker hole color (only used in separate output)")
  flag.Parse()
  fmt.Println("Separate:", *outputSeparateFiles)
  fmt.Println("Pretty:", *shouldIndent)
  fmt.Println("Skip markerless:", *skipMarkerlessFeatures)
  fmt.Println("Skip featureless:", *skipFeaturelessMarkers)
  fmt.Println("Exclude cell features:", *excludeCellFeatures)
  if *gridLevel > 0 {
    fmt.Println("Grid:", fmt.Sprintf("Level %d", *gridLevel))
  } else {
    fmt.Println("Grid: false")
  }
  fmt.Println("Max cell features:", *maxCellFeatures)
  fmt.Println("Max level:", *maxLevel)
  fmt.Println("Min level:", *minLevel)
  fmt.Println("Max cells:", *maxCells)
  fmt.Println("Markers:", *markerInputFilePath != "")
  fmt.Println("")
  // markerInputFileName := filepath.Base(*markerInputFilePath)
  os.MkdirAll(*outputDirectory, os.ModePerm)

  // Spaghetti for now

  boundingRect := s2.EmptyRect()

  var markers []Marker
  featuresWithMarkers := []*geojson.Feature{}
  if *markerInputFilePath != "" {
    markers = getMarkersFromCsv(*markerInputFilePath, *markerColor, *gridLevel)
  } else {
    markers = []Marker{}
  }

  var featureCollection geojson.FeatureCollection
  inputFileName := ""
  if len(flag.Args()) > 0 {
    inputFilePath := flag.Args()[0]
    inputFileName = filepath.Base(inputFilePath)
    featureCollection = *getFeatureCollectionFromGeojson(inputFilePath)
  } else {
    featureCollection = *geojson.NewFeatureCollection()
  }
  for index, feature := range featureCollection.Features {
    featureName := ""
    if feature.Properties["name"] != nil {
      featureName = feature.Properties["name"].(string)
    }
    relRole := "outer"
    relName := ""
    if feature.Properties["@relations"] != nil {
      relRole = feature.Properties["@relations"].([]interface{})[0].(map[string]interface{})["role"].(string)
      if feature.Properties["@relations"].([]interface{})[0].(map[string]interface{})["reltags"].(map[string]interface{})["name"] != nil {
        relName = feature.Properties["@relations"].([]interface{})[0].(map[string]interface{})["reltags"].(map[string]interface{})["name"].(string)
      }
    }
    if featureName != "" && relName != "" {
      featureName = fmt.Sprintf("%s %s", featureName, relName)
    } else if relName != "" {
      featureName = relName
    } else {
      featureName = "unnamed"
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

    var featureBoundingRect s2.Rect
    if *gridLevel > 0 {
      featureBoundingRect = covering.RectBound()
      boundingRect = boundingRect.Union(featureBoundingRect)
    }

    if len(cellIds) > *maxCellFeatures || len(holeCellIds) > *maxCellFeatures {
      fmt.Println("Skipping", getPathForFeature(feature), len(cellIds), len(holeCellIds))
      continue
    }

    feature.SetProperty("stroke", *featureColor)
    feature.SetProperty("fill", *featureColor)

    var cellFeature *geojson.Feature
    if len(cellIds) > 0 {
      cellFeature = geojson.NewMultiPolygonFeature(cellGeometry...)
      cellFeature.SetProperty("cellids", cellIds)
      cellFeature.SetProperty("stroke-width", 1)
      cellFeature.SetProperty("fill-opacity", 0.3)
      if isHole {
        cellFeature.SetProperty("stroke", *holeColor)
        cellFeature.SetProperty("fill", *holeColor)
      } else {
        cellFeature.SetProperty("stroke", *coverColor)
        cellFeature.SetProperty("fill", *coverColor)
      }
    }

    var holeCellFeature *geojson.Feature
    if len(holeCellIds) > 0 {
      holeCellFeature = geojson.NewMultiPolygonFeature(holeCellGeometry...)
      holeCellFeature.SetProperty("holecellids", holeCellIds)
      holeCellFeature.SetProperty("stroke", *holeColor)
      holeCellFeature.SetProperty("stroke-width", 1)
      holeCellFeature.SetProperty("fill", *holeColor)
      holeCellFeature.SetProperty("fill-opacity", 0.3)
    }

    containedMarkers, containedHoleMarkers, nearbyMarkers := checkContainedMarkerFeatures(covering, holeCovering, *maxLevel, isHole, feature, markers)

    if *outputSeparateFiles {
      var outputGeojsonData []byte
      var err error
      tempFeatureCollection := geojson.NewFeatureCollection()
      if *gridLevel > 0 {
        gridFeature := getGridFeatureFromRect(featureBoundingRect, *gridLevel)
        gridFeature.SetProperty("stroke-width", 1)
        gridFeature.SetProperty("fill-opacity", 0.2)
        tempFeatureCollection.AddFeature(gridFeature)
      }
      if len(cellIds) > 0 && ! *excludeCellFeatures {
        tempFeatureCollection.AddFeature(cellFeature)
      }
      if len(holeCellIds) > 0 && ! *excludeCellFeatures {
        tempFeatureCollection.AddFeature(holeCellFeature)
      }
      for _, marker := range containedMarkers {
        marker.feature.SetProperty("marker-color", *markerCoverColor)
        tempFeatureCollection.AddFeature(marker.feature)
      }
      for _, marker := range containedHoleMarkers {
        marker.feature.SetProperty("marker-color", *markerHoleColor)
        tempFeatureCollection.AddFeature(marker.feature)
      }
      if ! *skipFeaturelessMarkers {
        for _, marker := range nearbyMarkers {
          marker.feature.SetProperty("marker-color", *markerColor)
          tempFeatureCollection.AddFeature(marker.feature)
        }
      }
      tempFeatureCollection.AddFeature(feature)
      if *shouldIndent {
        outputGeojsonData, err = json.MarshalIndent(tempFeatureCollection, "", " ")
      } else {
        outputGeojsonData, err = tempFeatureCollection.MarshalJSON()
      }
      check(err)
      err = ioutil.WriteFile(fmt.Sprintf("%s/%s_%05d %s.geojson", *outputDirectory, strings.Replace(getPathForFeature(feature), "/", "_", -1), index + 1, featureName), outputGeojsonData, 0644)
      check(err)
    } else {
      if len(cellIds) > 0 && ! *excludeCellFeatures {
        featureCollection.AddFeature(cellFeature)
        if len(containedMarkers) > 0 || len(containedHoleMarkers) > 0 {
          featuresWithMarkers = append(featuresWithMarkers, cellFeature)
        }
      }
      if len(holeCellIds) > 0 && ! *excludeCellFeatures {
        if len(containedMarkers) > 0 || len(containedHoleMarkers) > 0 {
          featuresWithMarkers = append(featuresWithMarkers, holeCellFeature)
        }
      }
    }

    if len(containedMarkers) > 0 || len(containedHoleMarkers) > 0 {
      featuresWithMarkers = append(featuresWithMarkers, feature)
    }

    if (index + 1) % 1000 == 0 {
      fmt.Println(fmt.Sprintf("Parsed %d Features", index + 1))
    }
  }

  if ! *outputSeparateFiles {
    var outputGeojsonData []byte
    var err error
    if *skipMarkerlessFeatures {
      featureCollection.Features = featuresWithMarkers
    }
    for _, marker := range markers {
      if len(marker.feature.Properties["within"].([]string)) > 0 {
        marker.feature.SetProperty("marker-color", markerCoverColor)
      } else if *skipFeaturelessMarkers {
        continue
      }
      featureCollection.AddFeature(marker.feature)
    }
    if *gridLevel > 0 {
      cellIds := []s2.CellID{}
      for _, marker := range markers {
        cellIds = append(cellIds, *marker.cellId)
      }
      markersCellUnion := s2.CellUnion(cellIds)
      boundingRect = boundingRect.Union(markersCellUnion.RectBound())
      gridFeature := getGridFeatureFromRect(boundingRect, *gridLevel)
      gridFeature.SetProperty("stroke-width", 1)
      gridFeature.SetProperty("fill-opacity", 0.2)
      featureCollection.Features = append([]*geojson.Feature{gridFeature}, featureCollection.Features...)
    }
    if *shouldIndent {
      outputGeojsonData, err = json.MarshalIndent(featureCollection, "", " ")
    } else {
      outputGeojsonData, err = featureCollection.MarshalJSON()
    }
    if inputFileName != "" {
      err = ioutil.WriteFile(fmt.Sprintf("%s/%s.geojson", *outputDirectory, strings.TrimSuffix(inputFileName, filepath.Ext(inputFileName))), outputGeojsonData, 0644)
    } else {
      err = ioutil.WriteFile(fmt.Sprintf("%s/output.geojson", *outputDirectory), outputGeojsonData, 0644)
    }
    check(err)
  }

  outputContainedMarkersToCsv(markers, *outputDirectory)

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


func getMarkersFromCsv(csvFilename string, markerColor string, gridLevel int) []Marker {
  markers := []Marker{}
  for _, row := range readCsv(csvFilename) {
    var marker Marker
    name := row[0]
    lat, err := strconv.ParseFloat(row[1], 64)
    check(err)
    lng, err := strconv.ParseFloat(row[2], 64)
    check(err)
    latlng := s2.LatLngFromDegrees(lat, lng)
    cellId := s2.CellIDFromLatLng(latlng)
    marker.cellId = &cellId
    feature := geojson.NewPointFeature([]float64{lng, lat})
    if gridLevel > 0 {
      feature.SetProperty(fmt.Sprintf("level%dcellid", gridLevel), cellId.Parent(gridLevel).ToToken())
    }
    feature.SetProperty("name", name)
    feature.SetProperty("cellid", cellId.ToToken())
    feature.SetProperty("within", []string{})
    feature.SetProperty("marker-color", markerColor)
    marker.feature = feature
    markers = append(markers, marker)
  }
  return markers
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


func outputContainedMarkersToCsv(markers []Marker, outputDirectory string) {
  csvFile, err := os.Create(fmt.Sprintf("%s/markers_within_features.csv", outputDirectory))
  check(err)
  defer csvFile.Close()
  writer := csv.NewWriter(csvFile)
  defer writer.Flush()
  for _, marker := range markers {
    lat, lng := marker.feature.Geometry.Point[1], marker.feature.Geometry.Point[0]
    within := marker.feature.Properties["within"].([]string)
    if len(within) > 0 {
      err := writer.Write([]string{marker.feature.Properties["name"].(string), strconv.FormatFloat(lat, 'f', -1, 64), strconv.FormatFloat(lng, 'f', -1, 64)})
      check(err)
    }
  }
}


func checkContainedMarkerFeatures(
  coveringCellUnion *s2.CellUnion,
  holeCoveringCellUnion *s2.CellUnion,
  maxLevel int,
  isMainFeatureHole bool,
  coveringFeature *geojson.Feature,
  markers []Marker) (
    []Marker, []Marker, []Marker) {
  containedMarkers := []Marker{}
  containedHoleMarkers := []Marker{}
  nearbyMarkers := []Marker{}
  notContainedMarkers := []Marker{}

  for _, marker := range markers {
    if coveringCellUnion.ContainsCellID(*marker.cellId) {
      isHole := false
      withinText := getPathForFeature(coveringFeature)
      if holeCoveringCellUnion.ContainsCellID(*marker.cellId) || isMainFeatureHole {
        isHole = true
        withinText += " (hole)"
      }
      within := marker.feature.Properties["within"]
      within = append(within.([]string), withinText)
      marker.feature.SetProperty("within", within)
      if isHole {
        containedHoleMarkers = append(containedMarkers, marker)
      } else {
        containedMarkers = append(containedMarkers, marker)
      }
    } else {
      notContainedMarkers = append(notContainedMarkers, marker)
    }
  }

  boundingCap := coveringCellUnion.CapBound()
  boundingCap = boundingCap.Expanded(boundingCap.Radius() / 10)
  for _, marker := range notContainedMarkers {
    if boundingCap.ContainsPoint(marker.cellId.Point()) {
      nearbyMarkers = append(nearbyMarkers, marker)
    }
  }

  return containedMarkers, containedHoleMarkers, nearbyMarkers
}


func getGridFeatureFromRect(rect s2.Rect, gridLevel int) *geojson.Feature {
  regionCoverer := &s2.RegionCoverer{MaxLevel: gridLevel, MinLevel: gridLevel, MaxCells: 10}
  covering := regionCoverer.Covering(rect)
  _, cellGeometry := getGeojsonMultiPolygonFromCellUnion(covering)
  feature := geojson.NewMultiPolygonFeature(cellGeometry...)
  return feature
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
    ci, cg := getGeojsonMultiPolygonFromCellUnion(covering)
    cellIds = append(cellIds, ci...)
    cellGeometry = append(cellGeometry, cg...)
  }
  return &covering, cellIds, cellGeometry
}


func getGeojsonMultiPolygonFromCellUnion(cellUnion s2.CellUnion) ([]string, [][][][]float64) {
  var cellIds []string
  var cellGeometry [][][][]float64
  for _, cellId := range cellUnion {
    cellIds = append(cellIds, cellId.ToToken())
    cellGeometry = append(cellGeometry, getGeometryFromCellId(cellId))
  }
  return cellIds, cellGeometry
}


func getGeometryFromCellId(cellId s2.CellID) [][][]float64 {
  var cellGeometry [][][]float64
  cell := s2.CellFromCellID(cellId)
  vertices := [][]float64{}
  for k := 0; k < 5; k++ {
    vertex := cell.Vertex(k % 4)
    latlng := s2.LatLngFromPoint(vertex)
    vertices = append(vertices, []float64{float64(latlng.Lng.Degrees()), float64(latlng.Lat.Degrees())})
  }
  cellGeometry = [][][]float64{vertices}
  return cellGeometry
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
  loop := s2.LoopFromPoints(points)
  loop.Normalize()
  return loop
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
