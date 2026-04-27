# ============================================================================
# Chaos-Sec Frontend - Multi-Stage Dockerfile
# React + TypeScript + Vite
# ============================================================================

# ---------------------------------------------------------------------------
# Stage: base - Shared node setup
# ---------------------------------------------------------------------------
FROM node:18-alpine AS base

WORKDIR /app

RUN apk add --no-cache tini

ENTRYPOINT ["/sbin/tini", "--"]

# ---------------------------------------------------------------------------
# Stage: dependencies - Install node_modules
# ---------------------------------------------------------------------------
FROM base AS dependencies

COPY package.json package-lock.json* ./

RUN npm install --ignore-scripts && \
    npm cache clean --force

# ---------------------------------------------------------------------------
# Stage: development - Vite dev server with hot reload
# ---------------------------------------------------------------------------
FROM dependencies AS development

ENV NODE_ENV=development
ENV VITE_API_URL=http://localhost:8080/api/v1
ENV VITE_WS_URL=ws://localhost:8080/ws

COPY . .

EXPOSE 3000

CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0", "--port", "3000"]

# ---------------------------------------------------------------------------
# Stage: builder - Production build
# ---------------------------------------------------------------------------
FROM dependencies AS builder

ENV NODE_ENV=production

COPY . .

RUN npm run build && \
    ls -la dist/

# ---------------------------------------------------------------------------
# Stage: production - Nginx serving static files
# ---------------------------------------------------------------------------
FROM nginx:alpine AS production

LABEL maintainer="Chaos-Sec <dev@chaos-sec.io>"
LABEL description="Chaos-Sec Frontend Dashboard"
LABEL stage="production"

RUN apk add --no-cache tini curl

# Remove default nginx config and static files
RUN rm -f /etc/nginx/conf.d/default.conf && \
    rm -rf /usr/share/nginx/html/*

# Copy custom nginx configuration
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Copy built assets from builder stage
COPY --from=builder /app/dist /usr/share/nginx/html

# Create nginx cache directories
RUN mkdir -p /var/cache/nginx && \
    chown -R nginx:nginx /var/cache/nginx && \
    chown -R nginx:nginx /usr/share/nginx/html && \
    chown -R nginx:nginx /etc/nginx/conf.d

# Healthcheck: verify nginx is serving content
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -fsS http://localhost/health || exit 1

# Add a simple health endpoint by creating an empty file nginx can serve
RUN echo "ok" > /usr/share/nginx/html/health

# Run as non-root user
USER nginx

EXPOSE 80

ENTRYPOINT ["/sbin/tini", "--"]

CMD ["nginx", "-g", "daemon off;"]
