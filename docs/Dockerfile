FROM node:24-alpine

LABEL project=boot_operator

RUN apk add --no-cache bash

WORKDIR /app

EXPOSE 5173

ENTRYPOINT ["./hack/start-docs.sh"]

