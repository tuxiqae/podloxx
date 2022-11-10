export interface IPodTableData {
  connections: {
    [key: string]: {
      ip1: string;
      ip2: string;
      packSum: number;
    };
  };
  containerId: string;
  fileName: string;
  ip: string;
  localReceivedBytes: number;
  localTransmitBytes: number;
  namespace: string;
  node: string;
  packetsSum: number;
  podName: string;
  receivedBytes: number;
  recevedStartBytes: number;
  startTime: string;
  transmitBytes: number;
  transmitStartBytes: number;
  unknownBytes: number;
  records: {
    packetsSum: number;
    transmitBytes: number;
    receivedBytes: number;
    unknownBytes: number;
    localTransmitBytes: number;
    localReceivedBytes: number;
    timeStamp: Date;
  }[];
}
