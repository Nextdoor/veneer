# Copyright 2025 Nextdoor, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM golang:1.24.6-alpine AS builder
WORKDIR /app
COPY main.go .
RUN go mod init mock-exporter && \
    CGO_ENABLED=0 go build -o mock-exporter main.go

FROM alpine:latest
COPY --from=builder /app/mock-exporter /mock-exporter
EXPOSE 8080
ENTRYPOINT ["/mock-exporter"]
