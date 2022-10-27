import { RequestMethodEnum } from '@lox/enums/request-method.enum';
import pkg from '../../package.json';

declare global {
  interface Window {
    appVersion: string;
  }
}

export interface IEnvironment {
  stage: 'dev' | 'prod' | 'production' | 'local';
  production: boolean;
  version?: string;
  //! SERVICES ...
  baseUrl: string;
  statsService?: {
    method?: RequestMethodEnum;
    endPoint?: string;
    header?: any;
  };
}

export const baseEnvironment: IEnvironment = {
  stage: 'dev',
  production: false,
  version: window.appVersion ?? pkg.version,
  baseUrl: 'http://localhost:1337',
  statsService: {
    method: RequestMethodEnum.GET,
    endPoint: '/traffic',
    header: {
      // authorization: OVER AUTH INTERCEPTOR,
      contentType: 'application/json'
    }
  }
};
