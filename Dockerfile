FROM golang

RUN apt-get update \
    && apt-get install --yes --no-install-recommends openjdk-11-jre \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /go/src/wind-server

COPY . .

RUN go install -v ./...

ENV JAVA_HOME "/usr/lib/jvm/java-11-openjdk-armhf"

ENTRYPOINT ["wind-server"]
