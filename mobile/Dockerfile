# Development image for Expo Go (SDK 54). The mobile application itself
# still runs on Expo Go or a native build; this container only hosts Metro.
FROM node:20-bookworm-slim

WORKDIR /app

COPY package*.json ./
# Expo Router brings optional web peers into the dependency graph. Legacy peer
# resolution keeps the native Expo dependency set reproducible without adding
# react-dom to the mobile application.
RUN npm ci --legacy-peer-deps --no-audit --no-fund

COPY . .

RUN addgroup --system expo \
  && adduser --system --ingroup expo expo \
  && mkdir -p /home/expo \
  && chown -R expo:expo /app /home/expo

USER expo

ENV HOME=/home/expo
ENV EXPO_DEVTOOLS_LISTEN_ADDRESS=0.0.0.0

EXPOSE 8081

CMD ["npx", "expo", "start", "--host", "lan", "--port", "8081"]
