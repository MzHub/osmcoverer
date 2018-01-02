osmcoverer
==========

**osmcoverer** takes OpenStreetMap GeoJSON as input and outputs the same GeoJSON augmented with S2 cell approximation of the Features using RegionCoverer.

### How to get it

See [releases](https://github.com/MzHub/osmcoverer/releases) for pre-built binaries.

To build it yourself, get [go](https://golang.org/doc/install), then:

    go get github.com/mzhub/osmcoverer

This will place the osmcoverer binary in your workspace's ``bin`` directory.

### Usage

    osmcoverer [options] <input file>

For all available options:

    osmcoverer -h

Example:

    osmcoverer -separate -pretty=false -skipcells=1000 -outdir=output input.geojson

You can paste the output GeoJSON into [geojson.io](http://geojson.io) and add your own Marker Features.
