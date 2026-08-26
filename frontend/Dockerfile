ARG NODE_BUILDER_IMAGE
ARG FRONTEND_RUNTIME_IMAGE
FROM ${NODE_BUILDER_IMAGE} AS builder

WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run typecheck && npm run build

FROM ${FRONTEND_RUNTIME_IMAGE}

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /src/dist /usr/share/nginx/html
EXPOSE 8080
