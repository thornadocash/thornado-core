#!/usr/bin/env bash

set -e

GO_MOD_PACKAGE="gitlab.com/thorchain/thornode/v3"

echo "Generating gogo proto code"
cd proto/thorchain/v1
proto_dirs=$(find . -path -prune -o -name '*.proto' -print0 | xargs -0 -n1 dirname | sort | uniq)
for dir in $proto_dirs; do
  # shellcheck disable=SC2044
  for file in $(find "${dir}" -maxdepth 1 -name '*.proto'); do
    # this regex checks if a proto file has its go_package set to github.com/strangelove-ventures/poa/...
    # gogo proto files SHOULD ONLY be generated if this is false
    # we don't want gogo proto to run for proto files which are natively built for google.golang.org/protobuf
    if grep -q "option go_package" "$file" && grep -H -o -c "option go_package.*$GO_MOD_PACKAGE/api" "$file" | grep -q ':0$'; then
      echo "$file"
      buf generate --template buf.gen.gogo.yaml "$file"
    fi
  done
done

echo "Generating pulsar proto code"
buf generate --template buf.gen.pulsar.yaml

cd ..

cp -r $GO_MOD_PACKAGE/* ../../.
rm -rf gitlab.com

# Copy files over for dep injection
rm -rf ../../api && mkdir ../../api
mv thorchain ../../api/.
mv types ../../api/.
mv common ../../api/.
mv bifrost ../../api/.
