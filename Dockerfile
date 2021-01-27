FROM golang

RUN apt-get update && apt-get upgrade --yes \
 && apt-get install --no-install-recommends --yes cmake g++ openssl python3-dev ca-certificates gfortran perl openjdk-11-jre

ENV HTTP=https://software.ecmwf.int/wiki/download/attachments/45757960 \
  ECCODES=eccodes-2.19.1-Source
RUN cd /tmp && wget --output-document=${ECCODES}.tar.gz ${HTTP}/${ECCODES}.tar.gz?api=v2 \
  && tar -zxvf ${ECCODES}.tar.gz

RUN cd /tmp/${ECCODES} && mkdir build && cd build && cmake .. && make -j$(grep processor /proc/cpuinfo | wc -l) && make install

WORKDIR /go/src/wind-server

COPY . .

RUN go install -v ./...

ENV JAVA_HOME "/usr/lib/jvm/java-11-openjdk-armhf"

ENTRYPOINT ["wind-server"]
