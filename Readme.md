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

    osmcoverer -separate -pretty=false -maxcellfeatures=1000 -outdir=output input.geojson

You can also include a CSV file of markers to check for overlap within the OSM Features:

    osmcoverer -separate -markers=markers.csv input.geojson

This will include markers in the output GeoJSON along with information of which Features they overlap. A markers_within_features.csv file will also be generated for the results.

Input GeoJSON may even be omitted. For example visualize markers and a grid of level 10 S2 Cells:

    osmcoverer -markers=markers.csv -grid=10

The output GeoJSON can be visualized by pasting into [geojson.io](http://geojson.io).

Be careful with large datasets and don't set minlevel or grid level too low.
