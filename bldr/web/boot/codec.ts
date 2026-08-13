import {
  BootReport,
  type BootReport as BootReportMessage,
} from './report.pb.js'

// marshalBootReportJson encodes a BootReport as deterministic proto JSON.
export function marshalBootReportJson(report: BootReportMessage): string {
  return BootReport.toJsonString(report)
}
