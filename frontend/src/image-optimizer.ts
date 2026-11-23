const MAX_BYTES = 950 * 1024; // 950 KB to stay safely under Bluesky's 1 MB blob limit

export interface OptimizedImageResult {
  file: File;
  width: number;
  height: number;
  originalSize: number;
  optimizedSize: number;
  wasCompressed: boolean;
}

function loadImage(file: File): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file);
    const img = new Image();
    img.onload = () => {
      URL.revokeObjectURL(url);
      resolve(img);
    };
    img.onerror = () => {
      URL.revokeObjectURL(url);
      reject(new Error(`Failed to decode image "${file.name}"`));
    };
    img.src = url;
  });
}

function canvasToBlob(canvas: HTMLCanvasElement, mimeType: string, quality: number): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob(
      (blob) => {
        if (!blob) {
          reject(new Error('Canvas compression produced an empty image'));
          return;
        }
        resolve(blob);
      },
      mimeType,
      quality
    );
  });
}

/**
 * Ensures images exceeding 950 KB are automatically resized and JPEG-compressed on the client
 * via HTML5 Canvas so uploads never fail Bluesky's 1 MB limit.
 */
export async function optimizeImageForUpload(file: File): Promise<OptimizedImageResult> {
  const img = await loadImage(file);
  const origWidth = img.naturalWidth || img.width;
  const origHeight = img.naturalHeight || img.height;

  if (file.size <= MAX_BYTES) {
    return {
      file,
      width: origWidth,
      height: origHeight,
      originalSize: file.size,
      optimizedSize: file.size,
      wasCompressed: false,
    };
  }

  let maxDim = 2048;
  const qualities = [0.88, 0.78, 0.68, 0.58, 0.48];
  const baseName = file.name.replace(/\.[^.]+$/, '') || 'image';

  for (let dimAttempt = 0; dimAttempt < 4; dimAttempt++) {
    const scale = Math.min(1, maxDim / Math.max(origWidth, origHeight));
    const targetW = Math.max(1, Math.round(origWidth * scale));
    const targetH = Math.max(1, Math.round(origHeight * scale));

    const canvas = document.createElement('canvas');
    canvas.width = targetW;
    canvas.height = targetH;
    const ctx = canvas.getContext('2d');
    if (!ctx) {
      throw new Error('Could not acquire 2D canvas context');
    }
    ctx.drawImage(img, 0, 0, targetW, targetH);

    for (const quality of qualities) {
      const blob = await canvasToBlob(canvas, 'image/jpeg', quality);
      if (blob.size <= MAX_BYTES || (dimAttempt === 3 && quality === qualities[qualities.length - 1])) {
        const compressedFile = new File([blob], `${baseName}.jpg`, {
          type: 'image/jpeg',
          lastModified: Date.now(),
        });
        return {
          file: compressedFile,
          width: targetW,
          height: targetH,
          originalSize: file.size,
          optimizedSize: compressedFile.size,
          wasCompressed: true,
        };
      }
    }

    maxDim = Math.round(maxDim * 0.75);
  }

  return {
    file,
    width: origWidth,
    height: origHeight,
    originalSize: file.size,
    optimizedSize: file.size,
    wasCompressed: false,
  };
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
}
