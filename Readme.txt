Usage
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

The output GeoJSON can be visualized by pasting into http://geojson.io

Be careful with large datasets and don't set minlevel or grid level too low.
