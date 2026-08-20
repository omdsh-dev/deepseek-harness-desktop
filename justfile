default:
    just --list --list-submodules

test:
    go test ./...

clean:
    rm -rf target

install:
    go install ./cmd/dsh-web-desktopify

dep:
    go mod tidy

mod custom 'examples/custom/justfile'
mod official 'examples/official/justfile'
