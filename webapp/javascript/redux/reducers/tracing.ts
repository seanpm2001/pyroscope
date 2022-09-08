import { createSlice, PayloadAction } from '@reduxjs/toolkit';
import type { Profile } from '@pyroscope/models/src';
import { MergeOutput, mergeWithQueryID } from '@webapp/services/render';
import type { RootState } from '@webapp/redux/store';
import { RequestAbortedError } from '@webapp/services/base';
import { addNotification } from './notifications';
import { createAsyncThunk } from '../async-thunk';

type MergeMetadata = {
  appName: string;
  startTime: string;
  endTime: string;
  profilesLength: number;
};

type SingleView =
  | { type: 'pristine'; profile?: Profile; mergeMetadata?: MergeMetadata }
  | { type: 'loading'; profile?: Profile; mergeMetadata?: MergeMetadata }
  | {
      type: 'loaded';
      profile: Profile;
      mergeMetadata: MergeMetadata;
    }
  | {
      type: 'reloading';
      profile: Profile;
      mergeMetadata: MergeMetadata;
    };
// TODO

interface TracingState {
  queryID: string;
  maxNodes: string;
  refreshToken?: string;

  singleView: SingleView;
}

let singleViewAbortController: AbortController | undefined;

const initialState: TracingState = {
  queryID: '',
  maxNodes: '1024',

  singleView: { type: 'pristine' },
};

export const fetchSingleView = createAsyncThunk<
  MergeOutput,
  null,
  { state: { tracing: TracingState } }
>('tracing/singleView', async (_, thunkAPI) => {
  if (singleViewAbortController) {
    singleViewAbortController.abort();
  }

  singleViewAbortController = new AbortController();
  thunkAPI.signal = singleViewAbortController.signal;

  const state = thunkAPI.getState();
  const res = await mergeWithQueryID(state.tracing, singleViewAbortController);

  if (res.isOk) {
    return Promise.resolve(res.value);
  }

  if (res.isErr && res.error instanceof RequestAbortedError) {
    return thunkAPI.rejectWithValue({ rejectedWithValue: 'reloading' });
  }

  thunkAPI.dispatch(
    addNotification({
      type: 'danger',
      title: 'Failed to load single view data',
      message: res.error.message,
    })
  );

  return Promise.reject(res.error);
});

export const tracingSlice = createSlice({
  name: 'tracing',
  initialState,
  reducers: {
    setMaxNodes(state, action: PayloadAction<string>) {
      state.maxNodes = action.payload;
    },
    setQueryID(state, action: PayloadAction<string>) {
      state.queryID = action.payload;
    },
    refresh(state) {
      state.refreshToken = Math.random().toString();
    },
  },
  extraReducers: (builder) => {
    /*************************/
    /*      Single View      */
    /*************************/
    builder.addCase(fetchSingleView.pending, (state) => {
      switch (state.singleView.type) {
        // if we are fetching but there's already data
        // it's considered a 'reload'
        case 'reloading':
        case 'loaded': {
          state.singleView = {
            ...state.singleView,
            type: 'reloading',
          };
          break;
        }

        default: {
          state.singleView = { type: 'loading' };
        }
      }
    });

    builder.addCase(fetchSingleView.fulfilled, (state, action) => {
      state.singleView = {
        ...action.payload,
        mergeMetadata: action.payload.mergeMetadata,
        type: 'loaded',
      };
    });

    builder.addCase(fetchSingleView.rejected, (state, action) => {
      switch (state.singleView.type) {
        // if previous state is loaded, let's continue displaying data
        case 'reloading': {
          let type: SingleView['type'] = 'reloading';
          if (action.meta.rejectedWithValue) {
            type = (
              action?.payload as { rejectedWithValue: SingleView['type'] }
            )?.rejectedWithValue;
          } else if (action.error.message === 'cancel') {
            type = 'loaded';
          }
          state.singleView = {
            ...state.singleView,
            type,
          };
          break;
        }

        default: {
          // it failed to load for the first time, so far all effects it's pristine
          state.singleView = {
            type: 'pristine',
          };
        }
      }
    });
  },
});

export const selectTracingState = (state: RootState) => state.tracing;

export default tracingSlice.reducer;
export const { actions } = tracingSlice;