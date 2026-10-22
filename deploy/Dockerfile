FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY server /usr/local/bin/collabos-server
COPY collab /usr/local/bin/collab
COPY web /srv/web
ENV COLLAB_WEB_DIR=/srv/web COLLAB_ADDR=:8080 COLLAB_AUTO_MIGRATE=true
EXPOSE 8080
CMD ["/usr/local/bin/collabos-server"]
