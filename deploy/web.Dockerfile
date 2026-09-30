FROM node:24.15.0-alpine AS build

WORKDIR /src
COPY package.json package-lock.json ./
COPY apps/web/package.json apps/web/package.json
RUN npm ci --ignore-scripts --no-audit --no-fund
COPY apps/web apps/web
ARG VITE_API_BASE_URL=http://127.0.0.1:4173
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL
RUN npm run web:build

FROM nginx:1.28.0-alpine

COPY deploy/web-nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/apps/web/dist /usr/share/nginx/html
EXPOSE 4173
