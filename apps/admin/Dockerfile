FROM node:22-alpine AS build

WORKDIR /app/apps/admin
COPY apps/admin/package.json apps/admin/package-lock.json* ./
RUN npm install
COPY apps/admin/ ./
RUN npm run build

FROM nginx:1.27-alpine

COPY apps/admin/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/web/dist /usr/share/nginx/html

EXPOSE 80
