# - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -
# Stage 1
# - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -

FROM golang:1.14 as builder

ENV GO111MODULE=on

WORKDIR /app

COPY . .

RUN make local
# - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -
# Stage 2 
# - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -

FROM busybox

COPY --from=builder /app/bin/ /app/bin/

ENTRYPOINT ["/app/bin/EZPTT"]