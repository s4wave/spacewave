// CSS modules
declare module "*.module.css";
declare module "*.module.scss";

// File loader
declare module '*.png' {
  const value: string;
  export default value;
}
declare module '*.svg' {
  const value: string;
  export default value;
}

