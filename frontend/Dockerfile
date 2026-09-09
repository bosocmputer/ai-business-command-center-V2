FROM node:22-alpine AS build

WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts
COPY . .

ARG VITE_LINE_LIFF_ID
ENV VITE_LINE_LIFF_ID=${VITE_LINE_LIFF_ID}
RUN test -n "$VITE_LINE_LIFF_ID" \
    && npm run test:liff-config \
    && npm run liff:verify \
    && npm run typecheck \
    && npm test \
    && npm run build

FROM nginx:1.28-alpine

COPY deploy/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 8080
