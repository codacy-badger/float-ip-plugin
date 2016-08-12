FROM alpine
RUN apk add --no-cache iptables
ADD bin/float-ip /
RUN chmod +x float-ip
ENTRYPOINT ["/float-ip"]
