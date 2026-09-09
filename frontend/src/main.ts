import { createApp } from 'vue';
import PrimeVue from 'primevue/config';
import ConfirmationService from 'primevue/confirmationservice';
import ToastService from 'primevue/toastservice';
import Tooltip from 'primevue/tooltip';

import App from './App.vue';
import router from './router';
import { APP_THEME_PRESET } from '@/constants/themeConstants';
import '@/assets/tailwind.css';
import '@/assets/styles.scss';

const app = createApp(App);

app.use(router);
app.use(PrimeVue, {
  ripple: true,
  locale: {
    startsWith: 'ขึ้นต้นด้วย',
    contains: 'มีคำว่า',
    notContains: 'ไม่มีคำว่า',
    endsWith: 'ลงท้ายด้วย',
    equals: 'เท่ากับ',
    notEquals: 'ไม่เท่ากับ',
    noFilter: 'ไม่กรอง',
    lt: 'น้อยกว่า',
    lte: 'น้อยกว่าหรือเท่ากับ',
    gt: 'มากกว่า',
    gte: 'มากกว่าหรือเท่ากับ',
    dateIs: 'วันที่ตรงกับ',
    dateIsNot: 'วันที่ไม่ตรงกับ',
    dateBefore: 'ก่อนวันที่',
    dateAfter: 'หลังวันที่',
    clear: 'ล้าง',
    apply: 'ใช้ตัวกรอง',
    matchAll: 'ตรงทุกเงื่อนไข',
    matchAny: 'ตรงอย่างน้อยหนึ่งเงื่อนไข',
    addRule: 'เพิ่มเงื่อนไข',
    removeRule: 'ลบเงื่อนไข',
    emptyFilterMessage: 'ไม่พบข้อมูลตามตัวกรอง',
    emptySearchMessage: 'ไม่พบข้อมูลที่ค้นหา',
    emptySelectionMessage: 'ยังไม่ได้เลือกรายการ',
    searchMessage: 'พบ {0} รายการ',
    selectionMessage: 'เลือกแล้ว {0} รายการ',
    passwordPrompt: 'กรุณากรอกรหัสผ่าน',
    weak: 'รหัสผ่านยังไม่ปลอดภัย',
    medium: 'รหัสผ่านปลอดภัยปานกลาง',
    strong: 'รหัสผ่านปลอดภัยมาก',
    aria: {
      close: 'ปิด', previous: 'ก่อนหน้า', next: 'ถัดไป', navigation: 'การนำทาง',
      pageLabel: 'หน้า {page}', firstPageLabel: 'หน้าแรก', lastPageLabel: 'หน้าสุดท้าย',
      nextPageLabel: 'หน้าถัดไป', prevPageLabel: 'หน้าก่อนหน้า', rowsPerPageLabel: 'จำนวนรายการต่อหน้า',
      showFilterMenu: 'แสดงเมนูตัวกรอง', hideFilterMenu: 'ซ่อนเมนูตัวกรอง',
      filterOperator: 'ตัวดำเนินการตัวกรอง', filterConstraint: 'เงื่อนไขตัวกรอง',
      selectAll: 'เลือกทุกรายการ', unselectAll: 'ยกเลิกเลือกทุกรายการ',
      selectRow: 'เลือกแถว', unselectRow: 'ยกเลิกเลือกแถว',
      expandRow: 'ขยายแถว', collapseRow: 'ยุบแถว'
    }
  },
  theme: {
    preset: APP_THEME_PRESET,
    options: { darkModeSelector: '.app-dark', cssLayer: false }
  }
});
app.use(ToastService);
app.use(ConfirmationService);
app.directive('tooltip', Tooltip);
app.mount('#app');
