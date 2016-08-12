#!/bin/bash
go clean && godep go build --ldflags '-extldflags "-static"' -o bin/float-ip  
