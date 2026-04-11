FROM node:20-alpine AS builder

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci --prefer-offline

COPY . .
RUN npm run build

FROM nginx:1.27-alpine

COPY --from=builder /app/build /usr/share/nginx/html/chaos_zookoo

RUN printf 'server {\n\
    listen 80;\n\
    root /usr/share/nginx/html;\n\
\n\
    location /chaos_zookoo/ {\n\
        try_files $uri $uri/ /chaos_zookoo/index.html;\n\
    }\n\
\n\
    location = / {\n\
        return 301 /chaos_zookoo/;\n\
    }\n\
}\n' > /etc/nginx/conf.d/default.conf

EXPOSE 80
