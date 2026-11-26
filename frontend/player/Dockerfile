# syntax=docker/dockerfile:1.7

# ---------- build stage ----------
FROM node:24-alpine AS builder

WORKDIR /app

# Trust extra CA certs (TLS-inspecting proxy / antivirus) during npm ci.
# The "certs" context is docker/certs at the repo root (see compose
# additional_contexts); it is empty on normal machines.
COPY --from=certs / /certs/
RUN cat /certs/*.crt > /certs/bundle.pem 2>/dev/null || true
ENV NODE_EXTRA_CA_CERTS=/certs/bundle.pem

COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund

COPY . .
RUN npm run build

# ---------- runtime stage ----------
FROM nginx:1.27-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 80

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1/healthz >/dev/null || exit 1
