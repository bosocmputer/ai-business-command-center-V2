<template>
  <main class="min-h-screen grid place-items-center p-6 bg-surface-50 dark:bg-surface-950">
    <section class="card text-center max-w-md">
      <i class="pi pi-compass text-primary text-5xl" />
      <h1 class="text-2xl font-bold mt-5 mb-2">ไม่พบหน้าที่ต้องการ</h1>
      <p class="text-muted-color mb-6">ลิงก์อาจหมดอายุหรือที่อยู่ไม่ถูกต้อง</p>
      <Button label="กลับหน้าหลัก" icon="pi pi-home" as="router-link" to="/app" />
    </section>
  </main>
</template>
