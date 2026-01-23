FROM node:22-alpine AS build

ENV FORCE_COLOR=0
RUN corepack enable
WORKDIR /opt/docusaurus

COPY . /opt/docusaurus/

RUN npm ci

ARG MARKETING_URL

RUN --mount=type=secret,id=STRAPI_URL,env=STRAPI_URL \
    --mount=type=secret,id=STRAPI_API_TOKEN,env=STRAPI_API_TOKEN \
    npm run build

FROM nginxinc/nginx-unprivileged:stable-alpine AS prod
WORKDIR /opt/docusaurus

COPY ./config/nginx-default.conf /etc/nginx/templates/default.conf.template
COPY --chmod=755 ./config/15-default.envsh /docker-entrypoint.d/

COPY --from=build /opt/docusaurus/build /var/docusaurus

EXPOSE 8080
