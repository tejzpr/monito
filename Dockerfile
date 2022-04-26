FROM golang:1.18-alpine AS build
WORKDIR /app
ADD . /app
RUN apk add git
RUN echo "Starting Build" && \
    go build -a -tags netgo -buildmode=pie -trimpath && \
    mkdir -p /dist/app/config && \
    mv /app/monito /dist/app/monito && \
    mv /app/LICENSE /dist/app/LICENSE && \
    mv /app/config/config.sample.json /dist/app/config/config.json && \
    echo "Completed Build" 

FROM alpine
RUN apk --no-cache add ca-certificates \
    && rm -rf /var/cache/apk/*
COPY --from=build /dist/ /
ENV PATH="/app:${PATH}"
EXPOSE 8430
WORKDIR /app
CMD ["/app/monito"]