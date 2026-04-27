# ==============================================================================
# Stage 1: Dependencies (The Cache Layer)
# ==============================================================================
FROM node:20-alpine AS deps
WORKDIR /app

# 🛡️ Stability: Copy only lockfiles first to maximize Docker layer caching
COPY package.json package-lock.json ./

# 🛡️ Performance: Use 'npm ci' for strict, deterministic dependency installation
RUN npm ci

# ==============================================================================
# Stage 2: Builder (The Compilation Layer)
# ==============================================================================
FROM node:20-alpine AS builder
WORKDIR /app

# Copy cached node_modules from the deps stage
COPY --from=deps /app/node_modules ./node_modules
COPY . .

# 🛡️ SLA: Build the application. 
# SvelteKit will compile into a standalone Node server in the /build directory.
# We do NOT bake runtime secrets here; those are injected by docker-compose.
RUN npm run build

# 🛡️ Performance: Strip out devDependencies to shrink the final runtime image
RUN npm prune --omit=dev

# ==============================================================================
# Stage 3: Runtime (The Zero-Trust Execution Layer)
# ==============================================================================
FROM node:20-alpine AS runtime
WORKDIR /app

# 🛡️ Zero-Trust: Create a non-root user to run the application
# Alpine already includes a 'node' user, we just need to enforce its use.
ENV NODE_ENV=production
ENV PORT=3000

# Copy strictly what is needed to run the server, leaving source code behind
COPY --from=builder --chown=node:node /app/build ./build
COPY --from=builder --chown=node:node /app/node_modules ./node_modules
COPY --from=builder --chown=node:node /app/package.json ./

# 🛡️ Zero-Trust: Switch from the default 'root' to the restricted 'node' user
USER node

# Expose the standard Node.js port
EXPOSE 3000

# Start the compiled SvelteKit Node server
CMD ["node", "build"]
