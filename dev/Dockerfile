FROM lushdigital/docker-golang-dep:latest as builder

RUN mkdir -p /go/src/github.com/minchao/smsender

COPY ./ /go/src/github.com/minchao/smsender/

RUN	cd /go/src/github.com/minchao/smsender && \
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags '-w' -o bin/smsender

RUN apk --update --no-cache add make nodejs npm&& \
	npm install -g yarn && \
	cd /go/src/github.com/minchao/smsender/webroot && \
	make build 

FROM alpine
RUN apk --update --no-cache add ca-certificates && \
	addgroup -S smsender && adduser -S -g smsender smsender
RUN mkdir -p /smsender/config
COPY --from=builder /go/src/github.com/minchao/smsender/bin/smsender /smsender/
COPY --from=builder /go/src/github.com/minchao/smsender/config/config.default.yml /
COPY --from=builder /go/src/github.com/minchao/smsender/webroot/dist /smsender/webroot/dist/
COPY --from=builder /go/src/github.com/minchao/smsender/docker-entrypoint.sh /
RUN chown -R smsender:smsender /smsender
RUN chmod +x /docker-entrypoint.sh
USER smsender
ENTRYPOINT ["/bin/sh", "/docker-entrypoint.sh"]

EXPOSE 8080
