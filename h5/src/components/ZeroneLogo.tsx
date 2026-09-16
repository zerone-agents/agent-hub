import React from 'react';

interface ZeroneLogoProps {
  size?: number;
  className?: string;
  /** 圆角半径比例（0-1，相对 size），默认 0.28 */
  radiusRatio?: number;
}

/**
 * Zerone 品牌 Logo：深色圆角方块 + 白色「Z」字标 + 翡翠绿点缀。
 * Z = Zero to One，右下角绿点象征从 0 到 1 的那一步跃迁。
 */
export const ZeroneLogo: React.FC<ZeroneLogoProps> = ({ size = 32, className = '', radiusRatio = 0.28 }) => {
  const radius = Math.round(size * radiusRatio);
  return (
    <div
      className={`bg-neutral-900 flex items-center justify-center shadow-sm select-none ${className}`}
      style={{ width: size, height: size, borderRadius: radius }}
    >
      <svg
        viewBox="0 0 24 24"
        width={Math.round(size * 0.62)}
        height={Math.round(size * 0.62)}
        fill="none"
        aria-label="Zerone"
      >
        <path
          d="M5 4.5h14v3.1L10.2 17.4H19v3.1H5v-3.1l8.8-9.8H5V4.5z"
          fill="#ffffff"
        />
        <circle cx="19" cy="5.2" r="1.6" fill="#34d399" />
      </svg>
    </div>
  );
};

export default ZeroneLogo;
