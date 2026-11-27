# The train Step's image: Brush v0.3.0 on Debian's Mesa Vulkan drivers. Brush
# trains through Vulkan, so the container needs only a card's render node.
# Build it with:
#   docker build --network host -t spacewave-scene-train - < sdk/scene/capture/Dockerfile
FROM debian:trixie-slim
ARG BRUSH_URL=https://github.com/ArthurBrussee/brush/releases/download/v0.3.0/brush-app-x86_64-unknown-linux-gnu.tar.xz
ARG BRUSH_SHA256=4f0f9a8785d1951c62df26aae247c02c5bba32b00f40b06df4e1c9b867399e20
RUN apt-get update \
 && apt-get install -y --no-install-recommends mesa-vulkan-drivers libvulkan1 ca-certificates curl xz-utils \
 && curl -fsSL -o /tmp/brush.tar.xz "$BRUSH_URL" \
 && echo "$BRUSH_SHA256  /tmp/brush.tar.xz" | sha256sum -c - \
 && tar -xJf /tmp/brush.tar.xz -C /tmp \
 && install -m 0755 /tmp/brush-app-x86_64-unknown-linux-gnu/brush_app /usr/local/bin/brush_app \
 && rm -rf /tmp/brush* \
 && apt-get purge -y curl xz-utils && apt-get autoremove -y && rm -rf /var/lib/apt/lists/*
