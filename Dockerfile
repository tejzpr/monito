FROM golang:1.18-alpine AS build
WORKDIR /app
ADD . /app
RUN apk add gcc musl-dev upx git
RUN echo "Starting Build" && \
    CC=$(which musl-gcc) CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -tags netgo -buildmode=pie -trimpath --ldflags '-s -w -linkmode external -extldflags "-static"' && \
    upx --best --lzma ./monito && \
    ./monito v && \
    mkdir -p /dist/app/config && mkdir -p /dist/etc/ssl/certs/ && \
    mv /etc/ssl/certs/ca-certificates.crt /dist/etc/ssl/certs/ && \
    mv /app/monito /dist/app/monito && \
    mv /app/LICENSE /dist/app/LICENSE && \
    mv /app/config/config.sample.json /dist/app/config/config.json && \
    echo "Completed Build" 

FROM scratch
COPY --from=build /dist/ /
ENV PATH="/app:${PATH}"
EXPOSE 8430
CMD ["/app/monito"]