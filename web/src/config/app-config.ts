import { translate as swt } from "@/i18n/runtime";
import packageJson from "../../package.json";

const currentYear = new Date().getFullYear();

export const APP_CONFIG = {
  name: "APTEX",
  version: packageJson.version,
  copyright: `© ${currentYear}, APTEX by Afterglow Labs.`,
  meta: {
    get title() { return swt("interface.m2410"); },
    get description() { return swt("interface.m2411"); },
  },
};
