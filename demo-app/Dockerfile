FROM node:22-alpine

# Apply latest OS security patches
USER root
RUN apk upgrade --no-cache && \
    apk add --no-cache coreutils tini
USER node

WORKDIR /app

COPY --chown=node:node package*.json ./
RUN npm ci --omit=dev

COPY --chown=node:node server.js .

ENV NODE_ENV=production \
    PORT=8080

EXPOSE 8080

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["node", "server.js"]
