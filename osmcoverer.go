package main

import (
  "flag"
  "fmt"
  "os"
  "strings"
  "encoding/json"
  "io/ioutil"
  "path/filepath"
  "github.com/golang/geo/s2"
  "github.com/paulmach/go.geojson"
)


func main() {
  outputSeparateFiles := flag.Bool("separate", false, "Output Features into separate files")
  shouldIndent := flag.Bool("pretty", true, "Output pretty printend GeoJSON")
  skipCells := flag.Int("skipcells", 1000, "Skip features with more cells than this")
  maxLevel := flag.Int("maxlevel", 20, "MaxLevel setting for RegionCoverer")
  minLevel := flag.Int("minlevel", 5, "MinLevel setting for RegionCoverer")
  maxCells := flag.Int("maxcells", 1000, "MaxCells setting for RegionCoverer")
  outputDirectory := flag.String("outdir", "output", "Output directory")
  flag.Parse()
  fmt.Println("Separate:", *outputSeparateFiles)
  fmt.Println("Pretty:", *shouldIndent)
  fmt.Println("Skip cells:", *skipCells)
  fmt.Println("Max level:", *maxLevel)
  fmt.Println("Min level:", *minLevel)
  fmt.Println("Max cells:", *maxCells)
  fmt.Println("")
  inputFilePath := flag.Args()[0]
  inputFileName := filepath.Base(inputFilePath)
  featureCollection := readGeojson(inputFilePath)
  indent := ""
  if *shouldIndent {
    indent = " "
  }
  os.MkdirAll(*outputDirectory, os.ModePerm)
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
    cellIds, cellGeometry := getCoveringFromPolygons(polygons, isHole, *maxLevel, *minLevel, *maxCells)
    holeCellIds, holeCellGeometry := getCoveringFromPolygons(holePolygons, true, *maxLevel, *minLevel, *maxCells)

    if len(cellIds) > *skipCells || len(holeCellIds) > *skipCells {
      fmt.Println("Skipping", getPathForFeature(feature), len(cellIds), len(holeCellIds))
      continue
    }

    cellFeature := geojson.NewMultiPolygonFeature(cellGeometry...)
    holeCellFeature := geojson.NewMultiPolygonFeature(holeCellGeometry...)

    cellFeature.SetProperty("cellids", cellIds)
    cellFeature.SetProperty("stroke", "#008000")
    cellFeature.SetProperty("stroke-width", 1)
    cellFeature.SetProperty("stroke-opacity", 1)
    cellFeature.SetProperty("fill", "#80ff80")
    cellFeature.SetProperty("fill-opacity", 0.3)
    holeCellFeature.SetProperty("holecellids", holeCellIds)
    holeCellFeature.SetProperty("stroke", "#ff8080")
    holeCellFeature.SetProperty("stroke-width", 1)
    holeCellFeature.SetProperty("stroke-opacity", 1)
    holeCellFeature.SetProperty("fill", "#ff8080")
    holeCellFeature.SetProperty("fill-opacity", 0.3)

    if *outputSeparateFiles {
      tempFeatureCollection := geojson.NewFeatureCollection()
      tempFeatureCollection.AddFeature(cellFeature)
      tempFeatureCollection.AddFeature(holeCellFeature)
      tempFeatureCollection.AddFeature(feature)
      outputGeojsonData, err := json.MarshalIndent(tempFeatureCollection, "", indent)
      check(err)
      err = ioutil.WriteFile(fmt.Sprintf("%s/%s_%05d.geojson", *outputDirectory, strings.Replace(getPathForFeature(feature), "/", "_", -1), index + 1), outputGeojsonData, 0644)
      check(err)
    } else {
      featureCollection.AddFeature(cellFeature)
      featureCollection.AddFeature(holeCellFeature)
    }

  }

  if ! *outputSeparateFiles {
    outputGeojsonData, err := json.MarshalIndent(featureCollection, "", indent)
    check(err)
    err = ioutil.WriteFile(fmt.Sprintf("%s/%s.geojson", *outputDirectory, strings.TrimSuffix(inputFileName, filepath.Ext(inputFileName))), outputGeojsonData, 0644)
    check(err)
  }
  fmt.Println("Done")
}


func getCoveringFromPolygons(polygons []*s2.Polygon, isHole bool, maxLevel int, minLevel int, maxCells int) ([]string, [][][][]float64) {
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
  return cellIds, cellGeometry
}


func readGeojson(geojsonFilename string) *geojson.FeatureCollection {
  rawGeojsonData, err := ioutil.ReadFile(geojsonFilename)
  check(err)
  featureCollection, err := geojson.UnmarshalFeatureCollection(rawGeojsonData)
  check(err)
  return featureCollection
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
