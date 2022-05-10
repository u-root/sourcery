FROM scratch

COPY cpud /bin/cpud
COPY cpu /bin/cpu
COPY date /bin
COPY cat /bin
COPY lib64 /lib64
COPY root /root
COPY elvish /
COPY ls /

COPY dev /dev
COPY etc /etc
COPY go /go
COPY linux_amd64 /linux_amd64
COPY pkg /pkg
COPY src /src
COPY tmp /tmp

# Export necessary port
EXPOSE 23

# Command to run
ENTRYPOINT ["/elvish"]
