FROM golang:1.26 AS builder
ARG TARGETOS
ARG TARGETARCH
ENV GOPROXY="https://goproxy.cn,direct"

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY pkg/ pkg/
COPY tests/ tests/
COPY Makefile Makefile

# Build
RUN make build

## runtime image
FROM debian:bookworm-slim
ENV LANG=C.UTF-8
ENV TZ=Asia/Shanghai

# runtime dependencies
RUN set -eux; \
    echo '' > /etc/apt/sources.list.d/debian.sources; \
    echo 'deb http://mirrors.aliyun.com/debian/ bookworm main non-free non-free-firmware contrib' > /etc/apt/sources.list; \
    echo 'deb-src http://mirrors.aliyun.com/debian/ bookworm main non-free non-free-firmware contrib' >> /etc/apt/sources.list; \
    echo 'deb http://mirrors.aliyun.com/debian-security/ bookworm-security main' >> /etc/apt/sources.list; \
    echo 'deb-src http://mirrors.aliyun.com/debian-security/ bookworm-security main' >> /etc/apt/sources.list; \
    echo 'deb http://mirrors.aliyun.com/debian/ bookworm-updates main non-free non-free-firmware contrib' >> /etc/apt/sources.list; \
    echo 'deb-src http://mirrors.aliyun.com/debian/ bookworm-updates main non-free non-free-firmware contrib' >> /etc/apt/sources.list; \
    echo 'deb http://mirrors.aliyun.com/debian/ bookworm-backports main non-free non-free-firmware contrib' >> /etc/apt/sources.list; \
    echo 'deb-src http://mirrors.aliyun.com/debian/ bookworm-backports main non-free non-free-firmware contrib' >> /etc/apt/sources.list; \
	apt-get update; \
	apt-get install -y --no-install-recommends \
		ca-certificates \
		netbase \
		tzdata \
        curl \
        procps \
        net-tools \
        ipvsadm \
        iproute2 \
        iptables \
        kmod \
	; \
	rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /workspace/build/* /app/