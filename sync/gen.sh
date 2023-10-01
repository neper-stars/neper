#set -ex

../tools/bin/schema-generate -p sync -alwaysAcceptFalse \
    data-streams/schemas/neper/*.schema.json |
    goimports |
    gofmt >schemas.go
