interface ProjectedImageMeasureArgs {
  id: string;
  projectedUrl: string;
  expectedWidth: number;
  expectedHeight: number;
  deadlineMs: number;
}

interface ProjectedImageSample {
  projectedUrl: string;
  resourceEntryCount: 1;
  id: string;
  requestStartMs: number;
  responseStartMs: number;
  responseEndMs: number;
  loadMs: number;
  decodeMs: number;
  frameMs: number;
  displayReadyMs: number;
  naturalWidth: number;
  naturalHeight: number;
  transferSize: number;
  decodedBodySize: number;
  traced: false;
}

export default async function measureProjectedImage(
  args: ProjectedImageMeasureArgs,
): Promise<ProjectedImageSample> {
  const projectedUrl = new URL(args.projectedUrl, window.location.href).href;
  const image = new Image();
  image.alt = "";
  Object.assign(image.style, {
    position: "fixed",
    left: "0",
    top: "0",
    width: "1px",
    height: "1px",
    opacity: "0.001",
    pointerEvents: "none",
  });
  document.body.append(image);

  let sampleStart = 0;
  let loadMs = 0;
  const deadline = AbortSignal.timeout(args.deadlineMs);
  let abortDeadline: (() => void) | undefined;
  const deadlineExceeded = new Promise<never>((_, reject) => {
    abortDeadline = () =>
      reject(new Error(`projected image timed out: ${projectedUrl}`));
    deadline.addEventListener("abort", abortDeadline, { once: true });
  });
  const loaded = new Promise<void>((resolve, reject) => {
    image.addEventListener(
      "load",
      () => {
        loadMs = performance.now() - sampleStart;
        resolve();
      },
      { once: true },
    );
    image.addEventListener(
      "error",
      () =>
        reject(new Error(`projected image failed to load: ${projectedUrl}`)),
      { once: true },
    );
  });

  try {
    performance.clearResourceTimings();
    sampleStart = performance.now();
    const requestStartMs = performance.now() - sampleStart;
    image.src = projectedUrl;

    await Promise.race([loaded, deadlineExceeded]);
    await Promise.race([image.decode(), deadlineExceeded]);
    const decodeMs = performance.now() - sampleStart;
    if (
      image.naturalWidth !== args.expectedWidth ||
      image.naturalHeight !== args.expectedHeight
    ) {
      throw new Error(
        `projected image dimensions ${image.naturalWidth}x${image.naturalHeight}, expected ${args.expectedWidth}x${args.expectedHeight}`,
      );
    }

    await Promise.race([
      new Promise<void>((resolve) => {
        requestAnimationFrame(() => resolve());
      }),
      deadlineExceeded,
    ]);
    const frameMs = performance.now() - sampleStart;

    const entries = performance.getEntriesByName(projectedUrl, "resource");
    if (entries.length !== 1) {
      throw new Error(
        `projected image resource entries ${entries.length}, expected 1 for ${projectedUrl}`,
      );
    }
    const entry = entries[0] as PerformanceResourceTiming;
    const relativeTime = (value: number): number =>
      value === 0 ? 0 : value - sampleStart;

    return {
      id: args.id,
      requestStartMs,
      projectedUrl,
      resourceEntryCount: 1,
      responseStartMs: relativeTime(entry.responseStart),
      responseEndMs: relativeTime(entry.responseEnd),
      loadMs,
      decodeMs,
      frameMs,
      displayReadyMs: frameMs,
      naturalWidth: image.naturalWidth,
      naturalHeight: image.naturalHeight,
      transferSize: entry.transferSize,
      decodedBodySize: entry.decodedBodySize,
      traced: false,
    };
  } finally {
    if (abortDeadline) deadline.removeEventListener("abort", abortDeadline);
    image.remove();
  }
}
