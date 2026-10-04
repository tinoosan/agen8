FROM node:24-alpine3.22
RUN apk upgrade --no-cache libcrypto3 libssl3
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --omit=dev --ignore-scripts && npm audit --audit-level=low
COPY server.mjs transport.mjs dispatch.mjs dispatch-loop.mjs config.mjs healthcheck.mjs ./
USER 10001:10000
EXPOSE 8788
HEALTHCHECK --interval=30s --timeout=5s --start-period=40s --retries=3 CMD ["node", "healthcheck.mjs"]
CMD ["node", "server.mjs"]
